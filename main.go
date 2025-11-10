package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Nominee struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image,omitempty"` // квадратне зображення
	Audio string `json:"audio,omitempty"` // якщо трек — посилання на mp3/ogg
}

type Category struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Nominees []Nominee `json:"nominees"`
}

type Categories struct {
	Categories []Category `json:"categories"`
}

// votes[categoryID][nomineeID] = count
var (
	votesMu sync.RWMutex
	votes   = map[string]map[string]int{}
)

var allCategories Categories

var (
	templates *template.Template
)

func main() {
	// Load categories at startup
	if err := loadCategories(); err != nil {
		log.Fatalf("failed to load categories: %v", err)
	}

	// Prepare templates
	mustParseTemplates()

	// Initialize votes map
	votesMu.Lock()
	for _, c := range allCategories.Categories {
		if votes[c.ID] == nil {
			votes[c.ID] = map[string]int{}
		}
		for _, n := range c.Nominees {
			if _, ok := votes[c.ID][n.ID]; !ok {
				votes[c.ID][n.ID] = 0
			}
		}
	}
	votesMu.Unlock()

	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Pages
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/vote", voteHandler)
	mux.HandleFunc("/results", resultsHandler)

	// API
	mux.HandleFunc("/api/vote", voteAPIHandler)
	mux.HandleFunc("/api/category/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/category/")
		for _, c := range allCategories.Categories {
			if c.ID == id {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(c)
				return
			}
		}
		http.NotFound(w, r)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown to print results
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-shutdownCh
		fmt.Println("\nShutting down... printing results:\n")
		printResults()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	log.Println("Anteiku Awards listening on http://localhost:8080")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func mustParseTemplates() {
	t := template.New("").Funcs(template.FuncMap{
		"get": func(m map[string]any, key string) any {
			return m[key]
		},
	})

	var err error
	t, err = t.ParseFiles(
		filepath.Join("templates", "base.html"),
		filepath.Join("templates", "index.html"),
		filepath.Join("templates", "vote.html"),
		filepath.Join("templates", "results.html"),
	)
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}
	templates = t
}

func loadCategories() error {
	f, err := os.Open(filepath.Join("data", "categories.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	if err := d.Decode(&allCategories); err != nil {
		return err
	}
	return nil
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	render(w, "index_content", map[string]any{})
}

func voteHandler(w http.ResponseWriter, r *http.Request) {
	render(w, "vote_content", map[string]any{
		"Categories": allCategories.Categories,
	})
}

func resultsHandler(w http.ResponseWriter, r *http.Request) {
	votesMu.RLock()
	defer votesMu.RUnlock()
	// Build a structure that pairs categories with current counts and percentages
	type NomineeResult struct {
		ID      string
		Name    string
		Votes   int
		Percent int
	}
	type CatResult struct {
		ID       string
		Title    string
		Nominees []NomineeResult
	}
	var out []CatResult
	for _, c := range allCategories.Categories {
		cr := CatResult{ID: c.ID, Title: c.Title}
		total := 0
		for _, n := range c.Nominees {
			total += votes[c.ID][n.ID]
		}
		for _, n := range c.Nominees {
			v := votes[c.ID][n.ID]
			p := 0
			if total > 0 {
				p = int((float64(v) / float64(total)) * 100.0)
			}
			cr.Nominees = append(cr.Nominees, NomineeResult{
				ID:      n.ID,
				Name:    n.Name,
				Votes:   v,
				Percent: p,
			})
		}
		out = append(out, cr)
	}
	render(w, "results_content", map[string]any{"Results": out})
}

type voteRequest struct {
	CategoryID string `json:"categoryId"`
	NomineeID  string `json:"nomineeId"`
}

func voteAPIHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req voteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Validate category and nominee
	cat := findCategory(req.CategoryID)
	if cat == nil || !nomineeExists(cat, req.NomineeID) {
		http.Error(w, "invalid category or nominee", http.StatusBadRequest)
		return
	}

	// Check cookie for per-category vote prevention
	votedCats := parseVotedCookie(r)
	if slices.Contains(votedCats, req.CategoryID) {
		http.Error(w, "already voted for this category", http.StatusForbidden)
		return
	}

	// Record vote
	votesMu.Lock()
	if _, ok := votes[req.CategoryID]; !ok {
		votes[req.CategoryID] = map[string]int{}
	}
	votes[req.CategoryID][req.NomineeID]++
	votesMu.Unlock()

	// Update cookie: add category to voted list
	votedCats = append(votedCats, req.CategoryID)
	http.SetCookie(w, &http.Cookie{
		Name:     "av_voted",
		Value:    strings.Join(votedCats, ","),
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 365, // 1 year
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func parseVotedCookie(r *http.Request) []string {
	c, err := r.Cookie("av_voted")
	if err != nil || c.Value == "" {
		return []string{}
	}
	parts := strings.Split(c.Value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func findCategory(id string) *Category {
	for i := range allCategories.Categories {
		if allCategories.Categories[i].ID == id {
			return &allCategories.Categories[i]
		}
	}
	return nil
}

func nomineeExists(c *Category, nomineeID string) bool {
	for _, n := range c.Nominees {
		if n.ID == nomineeID {
			return true
		}
	}
	return false
}

func render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	merged := map[string]any{"Page": page}
	if m, ok := data.(map[string]any); ok {
		for k, v := range m {
			merged[k] = v
		}
	}

	if err := templates.ExecuteTemplate(w, "base.html", merged); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func printResults() {
	votesMu.RLock()
	defer votesMu.RUnlock()
	fmt.Println("=== Anteiku Awards Results ===")
	for _, c := range allCategories.Categories {
		fmt.Printf("\n%s\n", c.Title)
		for _, n := range c.Nominees {
			fmt.Printf("  - %s: %d\n", n.Name, votes[c.ID][n.ID])
		}
	}
}
