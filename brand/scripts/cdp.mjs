// Minimal headless-Chrome driver over the DevTools protocol (no dependencies),
// trimmed from motion/scripts/cdp.mjs: load an HTML string, wait for its
// fonts, screenshot it at an exact pixel size, optionally on a transparent
// ground.
import { spawn } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const CHROME = process.env.CHROME || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

export async function launch({ port = 9347 } = {}) {
	const profile = mkdtempSync(join(tmpdir(), 'spotussi-brand-'));
	const chrome = spawn(
		CHROME,
		[
			'--headless=new',
			`--remote-debugging-port=${port}`,
			`--user-data-dir=${profile}`,
			'--no-first-run',
			'--hide-scrollbars',
			'--force-color-profile=srgb',
			'about:blank'
		],
		{ stdio: 'ignore' }
	);
	let target;
	for (let i = 0; i < 50 && !target; i++) {
		try {
			const list = await (await fetch(`http://127.0.0.1:${port}/json`)).json();
			target = list.find((t) => t.type === 'page');
		} catch {
			// Chrome is still starting.
		}
		if (!target) await sleep(200);
	}
	if (!target) throw new Error('Chrome did not start (set CHROME to its binary)');
	const ws = new WebSocket(target.webSocketDebuggerUrl);
	await new Promise((r) => ws.addEventListener('open', r));
	let id = 0;
	const pending = new Map();
	ws.addEventListener('message', (e) => {
		const m = JSON.parse(e.data);
		if (m.id && pending.has(m.id)) {
			pending.get(m.id)(m);
			pending.delete(m.id);
		}
	});
	const send = (method, params = {}) =>
		new Promise((resolve, reject) => {
			const i = ++id;
			pending.set(i, (m) => (m.error ? reject(new Error(m.error.message)) : resolve(m.result)));
			ws.send(JSON.stringify({ id: i, method, params }));
		});
	await send('Page.enable');

	/** Render `html` at width×height CSS px and return a PNG buffer. */
	async function render(html, { width, height, dpr = 1, transparent = false }) {
		await send('Emulation.setDeviceMetricsOverride', {
			width,
			height,
			deviceScaleFactor: dpr,
			mobile: false
		});
		await send('Emulation.setDefaultBackgroundColorOverride', {
			color: transparent ? { r: 0, g: 0, b: 0, a: 0 } : { r: 255, g: 255, b: 255, a: 1 }
		});
		const { frameTree } = await send('Page.getFrameTree');
		await send('Page.setDocumentContent', { frameId: frameTree.frame.id, html });
		await send('Runtime.evaluate', {
			expression: 'document.fonts.ready.then(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))))',
			awaitPromise: true
		});
		const r = await send('Page.captureScreenshot', {
			format: 'png',
			clip: { x: 0, y: 0, width, height, scale: 1 }
		});
		return Buffer.from(r.data, 'base64');
	}

	const close = async () => {
		ws.close();
		const exited = new Promise((r) => chrome.once('exit', r));
		chrome.kill();
		await exited;
		rmSync(profile, { recursive: true, force: true, maxRetries: 5 });
	};
	return { render, close };
}
