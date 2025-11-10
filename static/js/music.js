// 1) під час завантаження — тянем стан з БД
window.addEventListener("DOMContentLoaded", async () => {
  try {
    const r = await fetch("/api/prefs/music");
    if (r.ok) {
      const { musicOn } = await r.json();
      audio.muted = !musicOn;
      // якщо muted — іконка
      const btn = document.getElementById("mute-toggle");
      if (btn) btn.textContent = audio.muted ? "🔇" : "🔈";
    }
  } catch {}
  const saved = parseFloat(localStorage.getItem("musicTime"));
  if (audio && !isNaN(saved)) audio.currentTime = saved;
  tryPlay();
});

// 2) при натисканні — шлемо у БД
btn?.addEventListener("click", async () => {
  const a = document.getElementById("bg-music");
  if (!a) return;
  a.muted = !a.muted;
  btn.textContent = a.muted ? "🔇" : "🔈";
  try { await fetch("/api/prefs/music", { method:"POST", headers:{'Content-Type':'application/json'}, body: JSON.stringify({ on: !a.muted }) }); } catch {}
});
