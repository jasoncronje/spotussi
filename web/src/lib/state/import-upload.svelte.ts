// The export upload in progress, shared by every upload control (Settings,
// the setup guide). It lives outside any component so moving around the app
// doesn't cancel an upload or lose track of its import, and so two controls
// never start two uploads.
//
// A batch is one or more files (the zip, or several Streaming_History JSON
// files): each is uploaded, then its job polled about once a second until
// it settles, then the next. The server queues jobs anyway; doing them one
// after another keeps the progress readable.
import { api } from '#lib/api/client.js';
import { ApiError } from '#lib/data/cache.js';
import { unwrap } from '#lib/data/queries.js';
import { refreshAll } from '#lib/data/resource.svelte.js';
import {
	acceptable,
	importProblem,
	settled,
	uploadExport,
	type ImportJob,
	type ImportProblem
} from '#lib/data/imports.js';

export type Phase =
	| { kind: 'idle' }
	| { kind: 'uploading'; sent: number; total: number }
	| { kind: 'processing'; job: ImportJob }
	| { kind: 'done' }
	| { kind: 'failed'; code: string; problem: ImportProblem; job?: ImportJob };

const POLL_MS = 1000;
const POLL_HIDDEN_MS = 5000;
/** Consecutive failed polls before giving up on a job. */
const POLL_TRIES = 20;

function fail(code: string, message?: string | null, job?: ImportJob): Phase {
	return { kind: 'failed', code, problem: importProblem(code, message), job };
}

/** Failures where sending the same file again is the fix. */
export const RETRYABLE: readonly string[] = ['network', 'upload_failed', 'interrupted', 'internal'];

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

class ImportUpload {
	phase = $state<Phase>({ kind: 'idle' });
	/** The batch being imported, and which file is current. */
	files = $state<File[]>([]);
	index = $state(0);
	/** Jobs of this batch that have settled, in order. */
	jobs = $state<ImportJob[]>([]);
	/** Set after a cancelled upload, cleared by the next one. */
	cancelled = $state(false);
	/** Bumped whenever the server's job list changed (a job was created or settled). */
	changes = $state(0);

	#abort: (() => void) | null = null;

	get busy() {
		return this.phase.kind === 'uploading' || this.phase.kind === 'processing';
	}

	get file(): File | undefined {
		return this.files[this.index];
	}

	/** Totals over the batch's settled jobs. */
	get totals() {
		let added = 0;
		let duplicate = 0;
		let read = 0;
		for (const j of this.jobs) {
			added += j.rows_new;
			duplicate += j.rows_duplicate;
			read += j.rows_total ?? j.rows_done;
		}
		return { added, duplicate, read };
	}

	async start(files: File[]) {
		if (this.busy || files.length === 0) return;
		this.files = files;
		this.jobs = [];
		const bad = files.findIndex((f) => !acceptable(f));
		if (bad >= 0) {
			this.index = bad;
			this.cancelled = false;
			this.phase = fail('unsupported_file');
			return;
		}
		await this.#run(0);
	}

	async #run(from: number) {
		this.cancelled = false;
		for (let i = from; i < this.files.length; i++) {
			this.index = i;
			const ok = await this.#one(this.files[i]!);
			if (!ok) return;
		}
		this.phase = { kind: 'done' };
	}

	/** Stops the upload in flight. An import already on the server keeps running. */
	cancel() {
		this.#abort?.();
	}

	/** Uploads the current file again, then the rest of the batch. */
	retry() {
		if (this.busy || !this.file) return;
		this.jobs = this.jobs.filter((j) => j.state === 'done');
		return this.#run(this.index);
	}

	/** Back to the drop zone. */
	reset() {
		if (this.busy) return;
		this.phase = { kind: 'idle' };
		this.files = [];
		this.jobs = [];
		this.index = 0;
	}

	async #one(file: File): Promise<boolean> {
		this.phase = { kind: 'uploading', sent: 0, total: file.size };
		const up = uploadExport(file, (sent, total) => {
			if (this.phase.kind === 'uploading') this.phase = { kind: 'uploading', sent, total };
		});
		this.#abort = up.abort;
		let job: ImportJob;
		try {
			job = await up.done;
		} catch (err) {
			const e = err as ApiError;
			if (e.code === 'aborted') {
				this.cancelled = true;
				this.phase = { kind: 'idle' };
			} else {
				this.phase = fail(e.code ?? '', e.message);
			}
			return false;
		} finally {
			this.#abort = null;
		}
		this.changes++;
		this.phase = { kind: 'processing', job };

		let misses = 0;
		while (!settled(job)) {
			await sleep(document.visibilityState === 'visible' ? POLL_MS : POLL_HIDDEN_MS);
			try {
				job = await unwrap(api.GET('/imports/{id}', { params: { path: { id: job.id } } }));
				misses = 0;
				this.phase = { kind: 'processing', job };
			} catch (err) {
				if ((err as ApiError).status === 404 || ++misses >= POLL_TRIES) {
					this.phase = fail('lost_contact');
					return false;
				}
			}
		}

		this.jobs = [...this.jobs, job];
		this.changes++;
		// History, the status panel and anything else on screen now include
		// the new streams (or, for a re-import, confirm nothing changed).
		refreshAll();
		if (job.state === 'failed') {
			this.phase = fail(job.error_code ?? '', job.error, job);
			return false;
		}
		return true;
	}
}

export const importUpload = new ImportUpload();
