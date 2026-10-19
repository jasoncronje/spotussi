// Range-switch timing harness (M7). Evaluate in a page of the app, then
// `await __suite(false)` measures a fixed sequence of range and Plays/Time
// switches. Per switch: time to the URL change, time until nothing is
// loading or dimmed (3 idle frames), long tasks and the worst frame gap.
// Run it in a visible or headless browser: a hidden tab throttles frames.
// scripts/perf/range-switch.sh drives it with agent-browser.
(() => {
  const raf = () => new Promise((r) => requestAnimationFrame(r));
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const busy = () =>
    !!document.querySelector('.progress, [aria-busy="true"], .opacity-55');
  window.__measure = async function (label, el, hover = false) {
    if (!el) return { label, error: "no element" };
    if (hover) {
      el.dispatchEvent(new PointerEvent("pointerenter"));
      await sleep(150);
    }
    const longs = [];
    const po = new PerformanceObserver((l) => {
      for (const e of l.getEntries()) longs.push(Math.round(e.duration));
    });
    po.observe({ type: "longtask" });
    let maxGap = 0;
    let last = performance.now();
    let running = true;
    const frame = (t) => {
      maxGap = Math.max(maxGap, t - last);
      last = t;
      if (running) requestAnimationFrame(frame);
    };
    requestAnimationFrame(frame);
    const href = location.href;
    const t0 = performance.now();
    el.click();
    let tUrl = null;
    while (performance.now() - t0 < 3000) {
      if (location.href !== href) {
        tUrl = performance.now() - t0;
        break;
      }
      await sleep(1);
    }
    let idle = 0;
    let tSettle = null;
    while (performance.now() - t0 < 15000) {
      await raf();
      if (busy()) {
        idle = 0;
        tSettle = null;
      } else {
        if (!idle) tSettle = performance.now() - t0;
        if (++idle >= 3) break;
      }
    }
    await sleep(200);
    running = false;
    po.disconnect();
    return {
      label,
      url: tUrl === null ? null : Math.round(tUrl),
      settle: Math.round(tSettle),
      longTasks: longs,
      maxFrameGap: Math.round(maxGap),
    };
  };
  window.__quick = (t) =>
    document.querySelector(`nav[aria-label="Time range"] a[title="${t}"]`);
  window.__metric = (m) =>
    [...document.querySelectorAll('[aria-label="View controls"] button')].find(
      (b) => b.textContent.trim() === m,
    );
  window.__suite = async function (hover) {
    const out = [];
    for (const t of [
      "Last 7 days",
      "Last 6 months",
      "Last 12 months",
      "All time",
    ])
      out.push(await __measure(t, __quick(t), hover));
    for (const m of ["Time", "Plays"])
      out.push(await __measure(`metric ${m} (all time)`, __metric(m), hover));
    out.push(await __measure("Last 4 weeks", __quick("Last 4 weeks"), hover));
    for (const m of ["Time", "Plays"])
      out.push(await __measure(`metric ${m} (4w)`, __metric(m), hover));
    out.push(await __measure("All time (cached)", __quick("All time"), hover));
    out.push(
      await __measure("Last 4 weeks (cached)", __quick("Last 4 weeks"), hover),
    );
    return out;
  };
  return "ok";
})();
