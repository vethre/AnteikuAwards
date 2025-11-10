// --- тихе різдвяне музло, яке стартує саме ---
(() => {
    const audio = document.getElementById("bg-music");
    if (!audio) return;

    audio.volume = 0; // спочатку повна тиша
    audio.loop = true;

    // якщо браузер дозволяє — стартуємо одразу
    const tryPlay = async () => {
        try {
        await audio.play();
        // плавно підняти гучність протягом 2 секунд
        let vol = 0;
        const fade = setInterval(() => {
            vol += 0.02;
            if (vol >= 0.2) {
            vol = 0.2;
            clearInterval(fade);
            }
            audio.volume = vol;
        }, 100);
        } catch (e) {
        // якщо блокнув автоплей — спробуємо після першої дії
        document.addEventListener("click", startManually);
        document.addEventListener("keydown", startManually);
        }
    };

    const startManually = async () => {
        document.removeEventListener("click", startManually);
        document.removeEventListener("keydown", startManually);
        try {
        await audio.play();
        audio.volume = 0.2;
        } catch {}
    };

    // відновлюємо позицію після переходу між сторінками
    window.addEventListener("DOMContentLoaded", () => {
        const saved = parseFloat(localStorage.getItem("musicTime"));
        if (audio && !isNaN(saved)) audio.currentTime = saved;
        tryPlay(); // намагаємось запустити
    });

    window.addEventListener("beforeunload", () => {
        localStorage.setItem("musicTime", audio.currentTime);
    });

    const btn = document.getElementById("mute-toggle");
    btn?.addEventListener("click", () => {
    const a = document.getElementById("bg-music");
    if (!a) return;
    a.muted = !a.muted;
    btn.textContent = a.muted ? "🔇" : "🔈";
    });
})();
