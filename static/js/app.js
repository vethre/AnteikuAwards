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
                : `<img loading="lazy" decoding="async" src="${n.image}" alt="${n.name}">`);
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
        try {
          const stRes = await fetch(`/api/vote/status?categoryId=${encodeURIComponent(data.id)}`);
          if (stRes.ok) {
            const st = await stRes.json();
            if (st.voted && st.nomineeId) {
              document.querySelectorAll(`.btn-vote[data-category="${data.id}"]`).forEach(b => {
                const nid = b.getAttribute('data-nominee');
                if (nid === st.nomineeId) {
                  b.textContent = 'Проголосовано ✓';
                  b.classList.add('voted');
                  b.disabled = false; // на неё можно навести и отменить
                } else {
                  b.disabled = true;  // другие заблокированы
                  b.classList.remove('voted');
                }
              });
            }
          }
        } catch (err) {
          console.warn('vote status check failed', err);
        }
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

    const isVoted = btn.classList.contains('voted');
    const isCancelMode = btn.classList.contains('btn-cancel-hover');

    // === Клик по "Отменить" ===
    if (isVoted && isCancelMode) {
      const original = btn.textContent;
      btn.disabled = true;
      btn.textContent = 'Отменяем...';

      try {
        const res = await fetch('/api/vote/cancel', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ categoryId })
        });

        if (!res.ok) {
          const text = await res.text();
          btn.disabled = false;
          btn.textContent = original;
          alert(text || 'Не удалось отменить голос');
          return;
        }

        // Сброс состояния всех кнопок категории
        document.querySelectorAll(`.btn-vote[data-category="${categoryId}"]`).forEach(b => {
          b.disabled = false;
          b.classList.remove('voted', 'btn-cancel-hover');
          b.textContent = 'Проголосовать';
        });
      } catch (err) {
        btn.disabled = false;
        btn.textContent = original;
        alert('Проблема сети');
      }
      return;
    }

    // Уже проголосовано, но не в режиме отмены — игнорим клик
    if (isVoted) {
      return;
    }

    // === Обычное голосование ===
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

        if (res.status === 401) {
          alert(text || 'Сначала войди через Telegram в шапке сайта 😉');
        } else {
          alert(text || 'Голосование не удалось');
        }
        return;
      }

      btn.textContent = 'Проголосовано ✓';
      document.querySelectorAll(`.btn-vote[data-category="${categoryId}"]`).forEach(b => {
        if (b === btn) {
          b.disabled = false;
          b.classList.add('voted');
        } else {
          b.disabled = true;
          b.classList.remove('voted');
        }
      });
      burst(btn);
    } catch (err) {
      btn.disabled = false;
      btn.textContent = original;
      alert('Проблема сети');
    }

      // ===== Hover по кнопке "Проголосовано" -> "Отменить" =====
    document.addEventListener('mouseenter', (e) => {
      const btn = e.target.closest('.btn-vote.voted');
      if (!btn) return;

      // если кто-то уже в cancel-режиме — не трогаем
      if (!btn.dataset.originalLabel) {
        btn.dataset.originalLabel = btn.textContent;
      }
      btn.textContent = 'Отменить';
      btn.classList.add('btn-cancel-hover');
    }, true);

    document.addEventListener('mouseleave', (e) => {
      const btn = e.target.closest('.btn-vote.voted');
      if (!btn) return;

      // вернём прежний текст
      const original = btn.dataset.originalLabel || 'Проголосовано ✓';
      btn.textContent = original;
      btn.classList.remove('btn-cancel-hover');
    }, true);
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
// Snow (лайт-версія)
(() => {
  const cvs = document.getElementById('snow');
  if (!cvs) return;
  const ctx = cvs.getContext('2d');
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  let W, H, flakes = [], running = true;

  function resize(){
    const dpr = Math.min(window.devicePixelRatio || 1, 1.5); // даунскейл
    W = cvs.width = Math.floor(window.innerWidth  * dpr);
    H = cvs.height = Math.floor(window.innerHeight * dpr);
    cvs.style.width  = '100%';
    cvs.style.height = '100%';
    const base = Math.floor((W/dpr)*(H/dpr)/18000);           // рідше
    const count = reduce ? 0 : Math.min(90, Math.max(30, base));
    flakes = Array.from({length: count}).map(()=>spawn());
  }
  function spawn(){
    const r = 1 + Math.random()*2;
    return { x: Math.random()*W, y: Math.random()*H, r, s: .4 + Math.random()*1, a: Math.random()*Math.PI*2, drift: .3 + Math.random()*0.8 };
  }
  function tick(){
    if (!running) return requestAnimationFrame(tick);
    ctx.clearRect(0,0,W,H);
    ctx.fillStyle = 'rgba(255,255,255,.9)';
    for (const f of flakes){
      f.y += f.s; f.x += Math.cos(f.a += 0.01)*f.drift;
      if (f.y > H + 10) { f.y = -10; f.x = Math.random()*W; }
      if (f.x < -10) f.x = W+10; if (f.x > W+10) f.x = -10;
      ctx.beginPath(); ctx.arc(f.x, f.y, f.r, 0, Math.PI*2); ctx.fill();
    }
    requestAnimationFrame(tick);
  }
  document.addEventListener('visibilitychange', () => { running = !document.hidden; });
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
})()});
