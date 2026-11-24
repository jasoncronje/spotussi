// Draws a recorded MCP session as a terminal still: the question, each
// spotussi tool call with its arguments and the start of its result, and the
// answer, all copied from a Claude Code stream-json transcript (nothing is
// written by hand). The client's own tool loading (ToolSearch) is left out.
//
//   claude -p "who did I listen to most in March?" --mcp-config mcp.json \
//     --output-format stream-json --verbose > run.jsonl
//   node scripts/mcp-terminal.mjs run.jsonl out.png   (motion/)
import { readFileSync, writeFileSync } from 'node:fs';
import { open, sleep } from './cdp.mjs';

const [file, out] = process.argv.slice(2);
if (!file || !out) throw new Error('usage: node scripts/mcp-terminal.mjs <run.jsonl> <out.png>');
const lines = readFileSync(file, 'utf8').trim().split('\n').map((l) => JSON.parse(l));
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
// Markdown tables are drawn with aligned columns, as a terminal renderer does.
const tables = (s) => s.replace(/(?:^\|.*\|$\n?)+/gm, (block) => {
  const rows = block.trim().split('\n').filter((r) => !/^\|[-| ]+\|$/.test(r)).map((r) => r.slice(1, -1).split('|').map((c) => c.trim()));
  const w = rows[0].map((_, i) => Math.max(...rows.map((r) => r[i].length)));
  const line = (r) => r.map((c, i) => (/^[\d.,]+$/.test(c) ? c.padStart(w[i]) : c.padEnd(w[i]))).join('   ');
  return [line(rows[0]), w.map((n) => '─'.repeat(n)).join('   '), ...rows.slice(1).map(line)].join('\n') + '\n';
});
const md = (s) => esc(tables(s)).replace(/\*\*(.+?)\*\*/g, '<b>$1</b>');

const question = process.env.QUESTION || 'who did I listen to most in March?';
const blocks = [];
for (const m of lines) {
  if (m.type !== 'assistant' && m.type !== 'user') continue;
  for (const c of m.message.content) {
    if (typeof c !== 'object') continue;
    if (c.type === 'tool_use' && c.name.startsWith('mcp__spotussi__')) {
      const name = c.name.replace('mcp__spotussi__', '');
      const args = Object.entries(c.input).map(([k, v]) => `${k}: ${JSON.stringify(v)}`).join(', ');
      blocks.push({ kind: 'call', id: c.id, html: `<span class="dot">⏺</span> <b>spotussi - ${name}</b> <span class="dim">(MCP)</span>(${esc(args)})` });
    } else if (c.type === 'tool_result') {
      const call = blocks.find((b) => b.id === c.tool_use_id);
      if (!call) continue;
      const text = (Array.isArray(c.content) ? c.content.map((x) => x.text ?? '').join('') : String(c.content)).replace(/\s+/g, ' ');
      call.html += `\n  <span class="dim">⎿  ${esc(text.slice(0, 118))}…</span>`;
    } else if (c.type === 'text' && m.type === 'assistant') {
      blocks.push({ kind: 'text', html: `<span class="dot w">⏺</span> ${md(c.text).replace(/\n/g, '\n  ')}` });
    }
  }
}

const html = `<!doctype html><meta charset="utf-8"><style>
body { margin: 0; background: #0b0d10; }
.win { margin: 40px; border-radius: 14px; background: #15181d; border: 1px solid #2a2f37; overflow: hidden; width: 1200px; box-shadow: 0 30px 80px #0008; }
.bar { height: 40px; background: #1c2026; display: flex; align-items: center; gap: 9px; padding: 0 16px; border-bottom: 1px solid #2a2f37; }
.bar i { width: 13px; height: 13px; border-radius: 50%; background: #3a3f47; display: block; }
.bar span { margin-left: 12px; color: #7d8590; font: 13px ui-monospace, Menlo, monospace; }
pre { margin: 0; padding: 26px 30px 30px; color: #d5d9de; font: 15.5px/1.55 ui-monospace, 'SF Mono', Menlo, monospace; white-space: pre-wrap; }
b { color: #fff; font-weight: 700; }
.dim { color: #7d8590; }
.dot { color: #1ed760; }
.dot.w { color: #e6e6e6; }
.prompt { color: #9aa4af; }
.q { background: #22272e; display: block; padding: 6px 10px; border-radius: 6px; color: #fff; }
</style><div class="win"><div class="bar"><i></i><i></i><i></i><span>claude — spotussi MCP (mock instance)</span></div><pre><span class="q"><span class="prompt">&gt;</span> ${esc(question)}</span>
${blocks.map((b) => b.html).join('\n\n')}
</pre></div>`;

const b = await open({ width: 1280, height: 900, dpr: 2, port: 9335 });
await b.send('Page.navigate', { url: 'data:text/html;base64,' + Buffer.from(html).toString('base64') });
await sleep(1500);
const h = await b.evaluate(`Math.ceil(document.querySelector('.win').getBoundingClientRect().bottom + 40)`);
writeFileSync(out, await b.shot({ x: 0, y: 0, width: 1280, height: h }));
b.close();
console.log(out, h);
