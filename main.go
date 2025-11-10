package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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

var db *sql.DB

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
	handler := cacheStatic(logRequests(mux))

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
	mux.HandleFunc("/api/prefs/music", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getMusicPref(w, r)
			return
		}
		if r.Method == http.MethodPost {
			setMusicPref(w, r)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/tg-auth", tgAuthHandler)

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is empty")
	}
	var err error
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		log.Fatal(err)
	}

	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS votes (
	id bigserial PRIMARY KEY,
	user_key text NOT NULL,
	category_id text NOT NULL,
	nominee_id text NOT NULL,
	ts timestamptz NOT NULL DEFAULT now(),
	UNIQUE (user_key, category_id)
	)`)
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS user_prefs (
	user_key text PRIMARY KEY,
	music_on boolean NOT NULL DEFAULT true,
	updated_at timestamptz NOT NULL DEFAULT now()
	)`)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
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

func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		next.ServeHTTP(w, r)
	})
}

func verifyTelegramAuth(values url.Values, botToken string) (map[string]string, bool) {
	data := make(map[string]string)
	for k := range values {
		if k == "hash" {
			continue
		}
		data[k] = values.Get(k)
	}
	// строим data_check_string
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(data[k])
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(b.String()))
	check := hex.EncodeToString(mac.Sum(nil))
	return data, strings.EqualFold(check, values.Get("hash"))
}

func tgAuthHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	data, ok := verifyTelegramAuth(r.Form, os.Getenv("TG_BOT_TOKEN"))
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tgID := data["id"]
	// Переприв’язуємо user_key до Telegram
	http.SetCookie(w, &http.Cookie{
		Name: "av_uid", Value: "tg_" + tgID, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 60 * 60 * 24 * 365,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func getOrSetUserKey(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("av_uid"); err == nil && c.Value != "" {
		return c.Value
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	uid := "anon_" + hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name: "av_uid", Value: uid, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 60 * 60 * 24 * 365,
	})
	return uid
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

func getMusicPref(w http.ResponseWriter, r *http.Request) {
	userKey := getOrSetUserKey(w, r)
	var on sql.NullBool
	_ = db.QueryRow(`SELECT music_on FROM user_prefs WHERE user_key=$1`, userKey).Scan(&on)
	val := true
	if on.Valid {
		val = on.Bool
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"musicOn": val})
}

func setMusicPref(w http.ResponseWriter, r *http.Request) {
	userKey := getOrSetUserKey(w, r)
	var body struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	_, _ = db.Exec(`
    INSERT INTO user_prefs(user_key, music_on) VALUES ($1,$2)
    ON CONFLICT (user_key) DO UPDATE SET music_on=EXCLUDED.music_on, updated_at=now()
  `, userKey, body.On)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
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
	userKey := getOrSetUserKey(w, r)

	// заборона повторного голосу на уровне БД (UNIQUE)
	_, err := db.Exec(`INSERT INTO votes(user_key, category_id, nominee_id) VALUES ($1,$2,$3)`,
		userKey, req.CategoryID, req.NomineeID)
	if err != nil {
		http.Error(w, "уже проголосовано", http.StatusForbidden)
		return
	}

	// (не обязательно, но если хочешь — можешь продолжать вести in-memory счётчик)
	votesMu.Lock()
	votes[req.CategoryID][req.NomineeID]++
	votesMu.Unlock()

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
