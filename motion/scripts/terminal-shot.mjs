// Draws a command's saved output as a terminal still, verbatim: the text is
// the log file's, only coloured (budget lines bold, "ok"/PASS green, "over"/
// FAIL red, p95 figures bright).
//
//   make bench BENCH_FACTOR=2.5 > bench.log 2>&1   (repo root)
//   node scripts/terminal-shot.mjs bench.log out.png "make bench BENCH_FACTOR=2.5"   (motion/)
import { readFileSync, writeFileSync } from 'node:fs';
import { open, sleep } from './cdp.mjs';

const [file, out, command = '', title = 'spotussi — performance benchmark, 500k-play reference dataset'] = process.argv.slice(2);
if (!file || !out) throw new Error('usage: node scripts/terminal-shot.mjs <log> <out.png> [command] [title]');
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const body = readFileSync(file, 'utf8').trimEnd().split('\n').map((l) => {
  let h = esc(l);
  if (/^budget /.test(l)) h = `<b>${h}</b>`.replace(/ ok$/, ' <span class="ok">ok</span>').replace(/ over$/, ' <span class="bad">over</span>');
  else if (/^(PASS|ok\s)/.test(l)) h = `<span class="ok">${h}</span>`;
  else if (/^(FAIL|---)/.test(l)) h = `<span class="bad">${h}</span>`;
  else if (/ p95 /.test(l)) h = h.replace(/(p95\s+[\d.]+ ms)/, '<span class="hi">$1</span>');
  else h = `<span class="dim">${h}</span>`;
  return h;
}).join('\n');

const html = `<!doctype html><meta charset="utf-8"><style>
body { margin: 0; background: #0b0d10; }
.win { margin: 40px; display: inline-block; border-radius: 14px; background: #15181d; border: 1px solid #2a2f37; overflow: hidden; box-shadow: 0 30px 80px #0008; }
.bar { height: 40px; background: #1c2026; display: flex; align-items: center; gap: 9px; padding: 0 16px; border-bottom: 1px solid #2a2f37; }
.bar i { width: 13px; height: 13px; border-radius: 50%; background: #3a3f47; display: block; }
.bar span { margin-left: 12px; color: #7d8590; font: 13px ui-monospace, Menlo, monospace; }
pre { margin: 0; padding: 22px 28px 26px; color: #c9d1d9; font: 14px/1.5 ui-monospace, 'SF Mono', Menlo, monospace; }
b { color: #fff; }
.dim { color: #7d8590; }
.hi { color: #fff; }
.ok { color: #1ed760; font-weight: 700; }
.bad { color: #ff6b6b; font-weight: 700; }
.cmd { color: #fff; }
.cmd::before { content: '$ '; color: #1ed760; }
</style><div class="win"><div class="bar"><i></i><i></i><i></i><span>${esc(title)}</span></div><pre>${command ? `<span class="cmd">${esc(command)}</span>\n` : ''}${body}</pre></div>`;

const b = await open({ width: 1400, height: 900, dpr: 2, port: 9336 });
await b.send('Page.navigate', { url: 'data:text/html;base64,' + Buffer.from(html).toString('base64') });
await sleep(1500);
const r = await b.evaluate(`(() => { const r = document.querySelector('.win').getBoundingClientRect(); return { w: Math.ceil(r.right + 40), h: Math.ceil(r.bottom + 40) }; })()`);
writeFileSync(out, await b.shot({ x: 0, y: 0, width: r.w, height: r.h }));
b.close();
console.log(out, r);
