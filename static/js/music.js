const audio = document.getElementById("bg-music");
const btn   = document.getElementById("mute-toggle");

// Автовідтворення: пробуємо грати при першій взаємодії
function tryPlay(){
  if (!audio) return;
  audio.play().catch(() => {/* браузер блочить до взаємодії — ок */});
}

window.addEventListener("DOMContentLoaded", async () => {
  if (!audio) return;
  // тягнемо стан із БД
  try {
    const r = await fetch("/api/prefs/music");
    if (r.ok) {
      const { musicOn } = await r.json();
      audio.muted = !musicOn;
    }
  } catch {}

  // відновлюємо позицію
  const saved = parseFloat(localStorage.getItem("musicTime"));
  if (!Number.isNaN(saved)) audio.currentTime = saved;

  // оновлюємо іконку
  if (btn) btn.textContent = audio.muted ? "🔇" : "🔈";

  // спроба програти
  tryPlay();
});

// клік по кнопці mute
btn?.addEventListener("click", async () => {
  if (!audio) return;
  audio.muted = !audio.muted;
  btn.textContent = audio.muted ? "🔇" : "🔈";
  try {
    await fetch("/api/prefs/music", {
      method:"POST",
      headers:{'Content-Type':'application/json'},
      body: JSON.stringify({ on: !audio.muted })
    });
  } catch {}
});

// зберігаємо позицію раз на кілька секунд
let saveT;
audio?.addEventListener("timeupdate", () => {
  if (saveT) return;
  saveT = setTimeout(() => {
    localStorage.setItem("musicTime", String(audio.currentTime || 0));
    saveT = null;
  }, 2000);
});
