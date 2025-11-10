document.addEventListener('DOMContentLoaded', () => {
  // ===== Reveal on scroll (залишаємо як було) =====
  const revealEls = document.querySelectorAll('.reveal, [data-stagger] > *');
  const io = new IntersectionObserver((entries) => {
    entries.forEach((e) => {
      if (e.isIntersecting) {
        const el = e.target;
        if (el.parentElement?.hasAttribute('data-stagger')) {
          const i = Array.from(el.parentElement.children).indexOf(el);
          el.style.transitionDelay = `${i * 60}ms`;
        }
        el.classList.add('revealed');
        io.unobserve(el);
      }
    });
  }, { threshold: 0.1 });
  revealEls.forEach(el => io.observe(el));

  // ===== Модалка =====
  const modal = document.getElementById("modal");
  const modalTitle = document.getElementById("modal-title");
  const modalNominees = document.getElementById("modal-nominees");

  // Делегування для відкриття модалки по категоріях
  document.addEventListener('click', async (e) => {
    const openBtn = e.target.closest('.main-btn, .cat-btn');
    if (!openBtn) return;
    console.log('Open category click', openBtn.dataset.category);

    const catId = openBtn.dataset.category;
    try {
      const res = await fetch(`/api/category/${catId}`);
      console.log('API status', res.status);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();

      modalTitle.textContent = data.title;

      if (data.locked) {
        modalNominees.innerHTML = `
          <article class="nominee-card">
            <img src="${data.cover || '/static/img/unknown.png'}" alt="Скоро" />
            <div class="info">
              <div class="nominee-name">Итоги будут объявлены позже</div>
            </div>
          </article>
        `;
      } else {
        modalNominees.innerHTML = data.nominees.map(n => {
          const isVideo = n.image && n.image.endsWith('.mp4');
          const media = n.audio
            ? `<audio controls preload="none" src="${n.audio}"></audio>`
            : (isVideo
                ? `<video controls preload="metadata" src="${n.image}" style="width:100%;border-radius:10px"></video>`
                : `<img src="${n.image}" alt="${n.name}">`);
          return `
            <article class="nominee-card">
              ${media}
              <div class="info">
                <div class="nominee-name">${n.name}</div>
                <button class="btn btn-vote" data-category="${data.id}" data-nominee="${n.id}">Проголосовать</button>
              </div>
            </article>
          `;
        }).join("");
      }

      modal.classList.remove('hidden');
    } catch (err) {
      console.error('Category load failed:', err);
      alert('Не удалось загрузить категорию');
    }
  });

  // Закриття модалки (хрестик або фон)
  document.addEventListener('click', (e) => {
    if (e.target.matches('.close-modal') || e.target.classList.contains('modal-backdrop')) {
      modal.classList.add('hidden');
    }
  });

  // ===== Голосування (делегування на динамічні кнопки) =====
  document.addEventListener('click', async (e) => {
    const btn = e.target.closest('.btn-vote');
    if (!btn) return;

    const categoryId = btn.getAttribute('data-category');
    const nomineeId  = btn.getAttribute('data-nominee');
    if (!nomineeId || !categoryId) return;

    if (['active-year','inactive-year'].includes(categoryId)) {
      alert('Голосование по этой категории закрыто. Итоги будут позже.');
      return;
    }
    btn.disabled = true;
    const original = btn.textContent;
    btn.textContent = 'Голосуем...';

    try {
      const res = await fetch('/api/vote', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ categoryId, nomineeId })
      });
      if (!res.ok) {
        const text = await res.text();
        btn.disabled = false;
        btn.textContent = original;
        alert(text || 'Голосование не удалось');
        return;
      }
      btn.textContent = 'Проголосовано ✓';
      document.querySelectorAll(`.btn-vote[data-category="${categoryId}"]`).forEach(b => {
        if (b !== btn) b.disabled = true;
        b.classList.add('voted');
      });
      burst(btn);
    } catch (err) {
      btn.disabled = false;
      btn.textContent = original;
      alert('Проблема сети');
    }
  });
});

function burst(anchor) {
  const colors = ['#d4af37','#f1d97a','#fff'];
  for (let i=0;i<10;i++) {
    const s = document.createElement('span');
    s.className = 'confetti';
    s.style.position = 'fixed';
    const rect = anchor.getBoundingClientRect();
    s.style.left = (rect.left + rect.width/2) + 'px';
    s.style.top = (rect.top + window.scrollY) + 'px';
    const size = 6 + Math.random()*6;
    s.style.width = s.style.height = size + 'px';
    s.style.background = colors[i%colors.length];
    s.style.borderRadius = '2px';
    s.style.pointerEvents = 'none';
    s.style.zIndex = 50;
    const dx = (Math.random() - .5) * 120;
    const dy = -60 - Math.random()*80;
    const rot = (Math.random()-0.5)*140;
    s.animate([
      { transform: 'translate(0,0) rotate(0deg)', opacity: 1},
      { transform: `translate(${dx}px, ${dy}px) rotate(${rot}deg)`, opacity: 0}
    ], { duration: 900 + Math.random()*400, easing: 'cubic-bezier(.2,.7,.3,1)', fill: 'forwards' });
    document.body.appendChild(s);
    setTimeout(() => s.remove(), 1400);
  }
}

// Snow
(() => {
  const cvs = document.getElementById('snow');
  if (!cvs) return;
  const ctx = cvs.getContext('2d');
  let W, H, flakes = [];

  function resize(){
    W = cvs.width = window.innerWidth;
    H = cvs.height = window.innerHeight;
    flakes = Array.from({length: Math.min(140, Math.floor(W*H/12000))}).map(()=>spawn());
  }
  function spawn(){
    const r = 1 + Math.random()*2.5;
    return {
      x: Math.random()*W,
      y: Math.random()*H,
      r,
      s: .6 + Math.random()*1.2,      // speed
      a: Math.random()*Math.PI*2,     // angle
      drift: .4 + Math.random()*1.1   // horizontal drift
    };
  }
  function tick(){
    ctx.clearRect(0,0,W,H);
    ctx.fillStyle = 'rgba(255,255,255,.9)';
    for (const f of flakes){
      f.y += f.s;
      f.x += Math.cos(f.a += 0.01)*f.drift;
      if (f.y > H + 10) { f.y = -10; f.x = Math.random()*W; }
      if (f.x < -10) f.x = W+10; if (f.x > W+10) f.x = -10;
      ctx.beginPath(); ctx.arc(f.x, f.y, f.r, 0, Math.PI*2); ctx.fill();
    }
    requestAnimationFrame(tick);
  }
  window.addEventListener('resize', resize, {passive:true});
  resize(); tick();
})();

// --- Parallax for hero assets ---
(() => {
  const logo = document.querySelector('.hero-logo');
  const cup  = document.querySelector('.hero-trophy');
  if (!logo || !cup) return;
  let x=0,y=0;
  window.addEventListener('mousemove', (e) => {
    const mx = (e.clientX / window.innerWidth  - .5) * 10; // -5..5
    const my = (e.clientY / window.innerHeight - .5) * 10;
    x = mx; y = my;
    logo.style.transform = `translate3d(${x/6}px, ${y/6}px, 0)`;   // мягче
    cup .style.transform = `translate3d(${x/3}px, ${y/3}px, 0)`;   // чуть сильнее
  }, {passive:true});
})();
