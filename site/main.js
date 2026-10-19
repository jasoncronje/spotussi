// spotussi.app: the small amount of script the page needs. Everything reads without it.

const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;

// Reveal once on entering the viewport; never leaves content hidden.
(() => {
  const targets = document.querySelectorAll('[data-reveal], .switch, .cta');
  if (!('IntersectionObserver' in window)) { targets.forEach((el) => el.classList.add('is-in')); return; }
  const io = new IntersectionObserver((entries) => {
    for (const e of entries) if (e.isIntersecting) { e.target.classList.add('is-in'); io.unobserve(e.target); }
  }, { rootMargin: '0px 0px -10% 0px', threshold: 0.08 });
  targets.forEach((el) => io.observe(el));
  addEventListener('load', () => setTimeout(() => {
    targets.forEach((el) => { const r = el.getBoundingClientRect(); if (r.top < innerHeight && r.bottom > 0) el.classList.add('is-in'); });
  }, 2500));
})();

// Features: each bento tile runs a small sketch of its feature. Tiles that
// tell a short story loop by toggling is-on; the scrolling ones run while
// on screen. Reduced motion shows every sketch in its finished state.
(() => {
  const bento = document.querySelector('.bento');
  if (!bento) return;
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;

  // The listening clock: 24 spokes of five lamps, lit by plays per hour.
  const clock = bento.querySelector('.clock');
  if (clock) {
    const lit = [1, 1, 0, 0, 0, 0, 1, 2, 2, 3, 3, 2, 3, 2, 2, 3, 3, 4, 5, 4, 4, 3, 2, 2];
    let svg = '';
    for (let h = 0; h < 24; h++) {
      const a = (h / 24) * Math.PI * 2 - Math.PI / 2; // midnight at the top, 6 PM on the left
      for (let k = 0; k < 5; k++) {
        const r = 54 + k * 10;
        const x = 100 + Math.cos(a) * r, y = 100 + Math.sin(a) * r;
        svg += `<circle cx="${x.toFixed(1)}" cy="${y.toFixed(1)}" r="3.2"${k < lit[h] ? ` class="lit" style="--d:${h}"` : ''}/>`;
      }
    }
    clock.innerHTML = svg;
  }

  // Stats over time: 60 days of stacked shares, drawn twice for a seamless scroll.
  const share = bento.querySelector('.share-track');
  if (share) {
    let seed = 7;
    const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647);
    const colours = ['#f2ab3e', '#5b8def', '#e0587a', '#2bb3bf'];
    let days = '';
    for (let d = 0; d < 60; d++) {
      const total = 0.15 + rnd() * 0.75;
      const parts = colours.map(() => rnd());
      const sum = parts.reduce((x, y) => x + y, 0);
      days += `<span class="day">${parts.map((p, i) => `<i style="--c:${colours[i]};--h:${((p / sum) * total).toFixed(3)}"></i>`).join('')}</span>`;
    }
    share.innerHTML = days + days;
  }

  const counters = (root, run) => root.querySelectorAll('[data-count]').forEach((el) => {
    const end = +el.dataset.count;
    const fmt = (v) => el.dataset.format === 'hm' ? `${Math.floor(v / 60)} h ${Math.round(v % 60)} min` : Math.round(v).toLocaleString('en-US');
    if (!run) { el.textContent = fmt(end); return; }
    const start = performance.now();
    const tick = (now) => {
      const k = Math.min(1, (now - start) / 1100);
      el.textContent = fmt(end * (1 - Math.pow(1 - k, 3)));
      if (k < 1) requestAnimationFrame(tick);
    };
    el.textContent = fmt(0);
    requestAnimationFrame(tick);
  });

  const arts = [...bento.querySelectorAll('.b-art')];
  if (reduced || !('IntersectionObserver' in window)) {
    arts.forEach((art) => { art.classList.add('is-on'); counters(art, false); });
    return;
  }
  const timers = new Map();
  const cycle = (art) => {
    art.classList.remove('is-on');
    void art.offsetWidth;
    art.classList.add('is-on');
    counters(art, true);
    timers.set(art, setTimeout(() => {
      art.classList.remove('is-on');
      timers.set(art, setTimeout(() => cycle(art), 700));
    }, +art.dataset.loop));
  };
  const io = new IntersectionObserver((entries) => {
    for (const e of entries) {
      const art = e.target;
      art.classList.toggle('is-vis', e.isIntersecting);
      if (!art.dataset.loop) continue;
      clearTimeout(timers.get(art));
      if (e.isIntersecting) timers.set(art, setTimeout(() => cycle(art), 300));
      else art.classList.remove('is-on');
    }
  }, { threshold: 0.4 });
  arts.forEach((art) => io.observe(art));
})();

// Hero: someone using the app, rendered with Remotion (motion/). Captions and stop
// dots follow the video's clock; times match `beats` in motion/src/Tour.tsx.
(() => {
  const reel = document.querySelector('.reel');
  if (!reel) return;
  const video = reel.querySelector('video');
  const caption = reel.querySelector('.reel-caption');
  const dots = reel.querySelector('.reel-dots');
  const pause = reel.querySelector('.reel-pause');
  const beats = [
    { at: 0, text: 'Time, plays, tracks and artists, each against the period before.' },
    { at: 3.5, text: 'Top lists as deep as your history goes, with rank movement.' },
    { at: 7, text: 'Every artist, album and track has a page of its own.' },
    { at: 10.5, text: 'Every play in order, with its source and how long it ran.' },
    { at: 14, text: 'Your listening split into sessions, longest first.' },
    { at: 17.5, text: 'Stats over time, for any range you pick.' },
  ];
  const HOME = 20.2; // Overview is clicked from stats and returns dark
  // The 4:5 cut serves phones and portrait screens; <source media> picks it at
  // load, and crossing the line reloads the reel at the same moment.
  const tall = matchMedia('(max-width: 760px), (max-aspect-ratio: 1/1)');
  const setPoster = () => { video.poster = tall.matches ? 'assets/hero-reel-phone-poster.webp' : 'assets/hero-reel-poster.webp'; };
  setPoster();
  tall.addEventListener('change', () => {
    const t = video.currentTime;
    const wasPaused = video.paused;
    setPoster();
    video.load();
    video.addEventListener('loadedmetadata', () => { video.currentTime = t; if (!wasPaused) video.play().catch(() => {}); }, { once: true });
  });
  let shown = 0;

  const buttons = beats.map((b, n) => {
    const el = document.createElement('button');
    el.type = 'button';
    el.setAttribute('aria-label', b.text);
    el.addEventListener('click', () => { video.currentTime = b.at + 0.01; show(n); });
    dots.append(el);
    return el;
  });

  const show = (n) => {
    buttons.forEach((b, k) => b.setAttribute('aria-current', String(k === n)));
    if (n === shown) return;
    shown = n;
    if (reduced) { caption.textContent = beats[n].text; return; }
    caption.classList.add('is-out');
    setTimeout(() => { caption.textContent = beats[shown].text; caption.classList.remove('is-out'); }, 260);
  };
  const beatAt = (t) => (t >= HOME ? 0 : beats.reduce((n, b, k) => (t >= b.at ? k : n), 0));
  video.addEventListener('timeupdate', () => show(beatAt(video.currentTime)));

  const setPlaying = (on) => {
    pause.setAttribute('aria-pressed', String(!on));
    on ? video.play().catch(() => {}) : video.pause();
  };
  pause.addEventListener('click', () => setPlaying(video.paused));
  document.addEventListener('visibilitychange', () => { if (document.hidden) video.pause(); else if (pause.getAttribute('aria-pressed') === 'false') video.play().catch(() => {}); });

  show(0);
  // Under reduced motion the reel holds on its poster (the app in its window, lit) until asked.
  if (reduced) { video.removeAttribute('autoplay'); video.preload = 'none'; video.load(); }
  setPlaying(!reduced);
})();

// Showcase: the Plays/Time switch re-ranks the list, as it does in the app.
(() => {
  const section = document.querySelector('.switch');
  if (!section) return;
  const word = section.querySelector('.switch-word');
  const list = section.querySelector('.ranks');
  const buttons = [...section.querySelectorAll('.seg button')];
  const data = {
    plays: [['Drake', 212, '212'], ['Kendrick Lamar', 196, '196'], ['Travis Scott', 141, '141'], ['Fred again..', 120, '120'], ['Arctic Monkeys', 104, '104']],
    time: [['Kendrick Lamar', 763, '12 h 43 min'], ['Drake', 747, '12 h 27 min'], ['Metallica', 559, '9 h 19 min'], ['Travis Scott', 510, '8 h 30 min'], ['Fred again..', 395, '6 h 35 min']],
  };
  const render = (mode) => {
    const rows = data[mode];
    const max = rows[0][1];
    list.innerHTML = rows.map(([name, v, label]) =>
      `<li><span class="r-name">${name}</span><span class="r-meter" style="--v:${(v / max).toFixed(3)}"></span><span class="r-val">${label}</span></li>`).join('');
  };
  const set = (mode) => {
    if (section.dataset.mode === mode) return;
    section.dataset.mode = mode;
    buttons.forEach((b) => b.setAttribute('aria-checked', String(b.dataset.value === mode)));
    word.textContent = mode;
    if (reduced) { render(mode); return; }
    list.classList.add('is-swapping');
    setTimeout(() => { render(mode); list.classList.remove('is-swapping'); }, 200);
  };
  // The section is pinned for two screens: the first half ranks by plays, the
  // second by time. The buttons scroll to their half.
  const span = () => section.offsetHeight - innerHeight;
  const half = (mode) => section.getBoundingClientRect().top + scrollY + span() * (mode === 'plays' ? 0.15 : 0.85);
  let ticking = false;
  const onScroll = () => {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => {
      ticking = false;
      if (span() <= 0) return;
      const p = -section.getBoundingClientRect().top / span();
      if (p > -0.2 && p < 1.2) set(p < 0.5 ? 'plays' : 'time');
    });
  };
  addEventListener('scroll', onScroll, { passive: true });
  const go = (mode) => {
    if (span() <= 0) { set(mode); return; }
    scrollTo({ top: half(mode), behavior: reduced ? 'auto' : 'smooth' });
  };
  buttons.forEach((b) => b.addEventListener('click', () => go(b.dataset.value)));
  section.querySelector('.seg').addEventListener('keydown', (e) => {
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) return;
    e.preventDefault();
    const next = section.dataset.mode === 'plays' ? 'time' : 'plays';
    go(next);
    buttons.find((b) => b.dataset.value === next).focus();
  });
})();

// History: from your player to your dashboard. The steps scroll past a
// pinned stage; the step at the reading line picks its scene, which plays
// once and holds its last frame. Reduced motion swaps straight to each
// finished frame.
(() => {
  const flow = document.querySelector('.flow');
  if (!flow) return;
  const steps = [...flow.querySelectorAll('.flow-step')];
  const scenes = [...flow.querySelectorAll('.fs-scene')];
  const tabs = [...flow.querySelectorAll('.fs-tabs i')];
  const stage = flow.querySelector('.flow-stage');
  const stacked = matchMedia('(max-width: 860px)');
  const anim = !reduced;
  flow.classList.add('is-live');
  if (anim) flow.classList.add('is-anim');

  // Counters run from data-from to data-count after data-delay, over data-run ms;
  // primed, they wait on data-from.
  const runs = new Map();
  const fmt = (el, v) => el.dataset.format === 'ms' ? `${Math.floor(v / 60)}:${String(Math.floor(v % 60)).padStart(2, '0')}`
    : el.dataset.format === 'hm' ? `${Math.floor(v / 60)} h ${Math.round(v % 60)} min`
    : Math.round(v).toLocaleString('en-US');
  const count = (scene, run, primed) => {
    const token = (runs.get(scene) || 0) + 1;
    runs.set(scene, token);
    scene.querySelectorAll('[data-count]').forEach((el) => {
      const end = +el.dataset.count, from = +(el.dataset.from || 0);
      if (!run) { el.textContent = fmt(el, primed ? from : end); return; }
      const ms = +(el.dataset.run || 700), linear = el.dataset.ease === 'linear';
      el.textContent = fmt(el, from);
      setTimeout(() => {
        const start = performance.now();
        const tick = (now) => {
          if (runs.get(scene) !== token) return;
          const k = Math.min(1, (now - start) / ms);
          el.textContent = fmt(el, from + (end - from) * (linear ? k : 1 - Math.pow(1 - k, 3)));
          if (k < 1) requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      }, +(el.dataset.delay || 0));
    });
  };

  let at = -1;
  const offs = new Map();
  const show = (n) => {
    if (n === at) return;
    at = n;
    steps.forEach((s, k) => s.classList.toggle('is-at', k === n));
    tabs.forEach((t, k) => t.classList.toggle('is-at', k === n));
    scenes.forEach((sc, k) => {
      clearTimeout(offs.get(sc));
      if (k === n) {
        sc.classList.add('is-shown');
        if (!anim) { sc.classList.add('is-on'); count(sc, false); return; }
        sc.classList.remove('is-on');
        count(sc, false, true);
        void sc.offsetWidth;
        requestAnimationFrame(() => { if (at === k) { sc.classList.add('is-on'); count(sc, true); } });
      } else if (sc.classList.contains('is-shown')) {
        sc.classList.remove('is-shown');
        offs.set(sc, setTimeout(() => sc.classList.remove('is-on'), 480));
      }
    });
  };

  // The reading line: mid-screen beside the stage, or mid-way down the
  // space under it when the stage pins along the top.
  const pick = () => {
    const s = stage.getBoundingClientRect();
    if (s.top > innerHeight - s.height * 0.85 || s.bottom < innerHeight * 0.2) return; // most of the stage on screen
    const line = stacked.matches ? (s.bottom + innerHeight) / 2 : innerHeight / 2;
    let n = 0, best = Infinity;
    steps.forEach((st, k) => {
      const r = st.getBoundingClientRect(), d = Math.abs(r.top + r.height / 2 - line);
      if (d < best) { best = d; n = k; }
    });
    show(n);
  };
  // Before the section is reached, the first scene waits on its opening frame.
  scenes[0].classList.add('is-shown');
  if (anim) count(scenes[0], false, true); else show(0);
  let ticking = false;
  const onScroll = () => {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => { ticking = false; pick(); });
  };
  addEventListener('scroll', onScroll, { passive: true });
  addEventListener('resize', onScroll);
  pick();
})();

// Installation: one stage open at a time. A stage's sketch plays once, the
// first time the stage is open on screen, and then holds its last frame. The
// sketches ship finished, so without script or with reduced motion they hold
// that frame from the start.
(() => {
  const box = document.querySelector('.stages');
  if (!box) return;
  const stages = [...box.querySelectorAll('.stage')];
  const arts = stages.map((s) => s.querySelector('.s-art'));
  let open = stages.find((s) => s.classList.contains('is-open'));
  let visible = false;
  const played = new Set();
  const animate = !reduced && 'IntersectionObserver' in window;

  // Side by side, the row is as tall as the tallest stage, so switching
  // stages never changes its height. Measured with every sketch finished.
  const wide = matchMedia('(min-width: 1101px)');
  const fit = () => {
    box.style.minHeight = '';
    if (!wide.matches) return;
    const fresh = arts.filter((a) => !a.classList.contains('is-on'));
    box.classList.add('is-measure');
    fresh.forEach((a) => a.classList.add('is-on'));
    box.style.minHeight = `${Math.max(...stages.map((s) => s.querySelector('.stage-body').scrollHeight))}px`;
    fresh.forEach((a) => a.classList.remove('is-on'));
    void box.offsetWidth;
    box.classList.remove('is-measure');
  };
  fit();
  let resizing = 0;
  addEventListener('resize', () => { cancelAnimationFrame(resizing); resizing = requestAnimationFrame(fit); });
  document.fonts?.ready.then(fit);

  const counters = (root) => root.querySelectorAll('[data-count]').forEach((el) => {
    const end = +el.dataset.count, dur = +(el.dataset.dur || 1200);
    const t0 = performance.now() + +(el.dataset.delay || 0);
    const tick = (now) => {
      const k = Math.max(0, Math.min(1, (now - t0) / dur));
      el.textContent = Math.round(end * k).toLocaleString('en-US');
      if (k < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  const play = () => {
    const stage = open;
    const a = stage.querySelector('.s-art');
    if (!visible || played.has(a)) return;
    played.add(a);
    // Wait for the stage body to fade in.
    setTimeout(() => { void a.offsetWidth; a.classList.add('is-on', 'is-vis'); counters(a); }, 350);
  };

  stages.forEach((stage) => {
    stage.querySelector('.stage-tab').addEventListener('click', () => {
      open = stage;
      stages.forEach((s) => {
        const on = s === stage;
        s.classList.toggle('is-open', on);
        s.querySelector('.stage-tab').setAttribute('aria-expanded', String(on));
      });
      // The tab hides once its stage is open; keep focus on the stage it opened.
      stage.querySelector('h3').focus({ preventScroll: true });
      if (animate) play();
    });
  });

  if (!animate) return;
  arts.forEach((a) => a.classList.remove('is-on'));
  new IntersectionObserver(([e]) => { visible = e.isIntersecting; play(); }, { threshold: 0.3 }).observe(box);
})();

// FAQ: cards in a drawer, one open at a time.
(() => {
  const cards = [...document.querySelectorAll('.card')];
  cards.forEach((card) => {
    card.querySelector('.card-q').addEventListener('click', () => {
      const opening = !card.classList.contains('is-open');
      cards.forEach((c) => {
        const open = c === card && opening;
        c.classList.toggle('is-open', open);
        c.querySelector('.card-q').setAttribute('aria-expanded', String(open));
      });
    });
  });
})();

// Figures: one small sketch per tile, run only while on screen. The range
// switch redraws its chart at once; the ring counts the 12 s read (four
// times faster) and the app catches up with the player on the read; the
// board's status lamp ticks over. The HTML holds the finished frames, which
// is what reduced motion and no-script readers see.
(() => {
  const section = document.querySelector('.figures');
  if (!section || reduced || !('IntersectionObserver' in window)) return;

  const range = section.querySelector('.fx-range');
  const runRange = (fx) => {
    const tabs = [...fx.querySelectorAll('.rg-seg span')];
    const cols = [...fx.querySelectorAll('.rg-chart span')];
    const finished = cols.map((c) => c.style.getPropertyValue('--n'));
    const sets = [
      [3, 5, 2, 7, 9, 4, 6, 3, 8, 11, 5, 2, 4, 7, 10, 6, 3, 5, 9, 12, 7, 4, 6, 8],
      [6, 8, 7, 9, 10, 8, 7, 6, 9, 11, 10, 8, 7, 9, 12, 10, 8, 7, 9, 10, 8, 6, 7, 9],
      finished,
    ];
    let k = 2, timer;
    const show = (i) => { tabs.forEach((t, j) => t.classList.toggle('is-on', j === i)); cols.forEach((c, j) => c.style.setProperty('--n', sets[i][j])); };
    const step = () => { k = (k + 1) % 3; show(k); timer = setTimeout(step, 1800); };
    return { start: () => { timer = setTimeout(step, 900); }, stop: () => { clearTimeout(timer); k = 2; show(2); } };
  };

  const sync = section.querySelector('.fx-sync');
  const runSync = (fx) => {
    const ring = fx.querySelector('.sy-ring');
    const lamps = [...ring.querySelectorAll('circle')];
    const play = fx.querySelector('.sy-play');
    const app = fx.querySelector('.sy-app');
    const songs = ['Passionfruit', 'Arabella', 'Nothing Else Matters'];
    let n = 3, tick = 0, playing = 1, known = 1, timer;
    const light = () => { lamps.forEach((l, i) => l.classList.toggle('lit', i < n)); };
    const step = () => {
      tick++;
      n = n % 12 + 1;
      light();
      ring.classList.toggle('is-read', n === 12);
      app.classList.remove('is-new');
      // The player moves on partway through a read; the read catches it.
      if (n === 6 && Math.floor(tick / 12) % 2 === 0) { playing = (playing + 1) % songs.length; play.textContent = songs[playing]; }
      if (n === 12 && known !== playing) { known = playing; app.textContent = songs[known]; app.classList.add('is-new'); }
      timer = setTimeout(step, 250);
    };
    return {
      start: () => { timer = setTimeout(step, 400); },
      stop: () => { clearTimeout(timer); n = 3; light(); ring.classList.remove('is-read'); app.classList.remove('is-new'); play.textContent = app.textContent = songs[playing = known = 1]; },
    };
  };

  const board = section.querySelector('.board');
  const runBoard = (fx) => ({ start: () => fx.classList.add('is-vis'), stop: () => fx.classList.remove('is-vis') });

  const loops = new Map([[range, runRange], [sync, runSync], [board, runBoard]].filter(([fx]) => fx).map(([fx, make]) => [fx, make(fx)]));
  const io = new IntersectionObserver((entries) => {
    for (const e of entries) {
      const loop = loops.get(e.target);
      loop.stop();
      if (e.isIntersecting) loop.start();
    }
  }, { threshold: 0.4 });
  loops.forEach((_, fx) => io.observe(fx));
})();
