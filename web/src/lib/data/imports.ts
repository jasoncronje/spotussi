// Export uploads (D23). The upload itself goes through XMLHttpRequest, the
// one call in the app that does: fetch can't report upload progress, and an
// export is 20–300 MB. It sends what the generated client would (session
// cookie, X-CSRF-Token) and rejects with the same ApiError shape.
import { csrfHeader, reportUnauthorized, type Schemas } from '#lib/api/client.js';
import { ApiError } from './cache.js';

export type ImportJob = Schemas['ImportJob'];

export const ACCEPT = '.zip,.json,application/zip,application/json';

/** Whether a picked or dropped file looks like something the server takes. */
export function acceptable(file: File) {
	return /\.(zip|json)$/i.test(file.name);
}

export function settled(job: ImportJob) {
	return job.state === 'done' || job.state === 'failed';
}

export type Upload = { done: Promise<ImportJob>; abort: () => void };

/** POST /imports with the file as the body, reporting bytes sent. */
export function uploadExport(
	file: File,
	onProgress: (sent: number, total: number) => void
): Upload {
	const xhr = new XMLHttpRequest();
	const done = new Promise<ImportJob>((resolve, reject) => {
		xhr.open('POST', `/api/v1/imports?filename=${encodeURIComponent(file.name)}`);
		for (const [k, v] of Object.entries(csrfHeader())) xhr.setRequestHeader(k, v);
		xhr.setRequestHeader(
			'Content-Type',
			file.type || (/\.zip$/i.test(file.name) ? 'application/zip' : 'application/json')
		);
		xhr.upload.onprogress = (e) => onProgress(e.loaded, e.lengthComputable ? e.total : file.size);
		xhr.onload = () => {
			let body: unknown = null;
			try {
				body = JSON.parse(xhr.responseText);
			} catch {
				// not JSON (a proxy's error page): fall through to the status
			}
			if (xhr.status === 202 || xhr.status === 200) {
				resolve(body as ImportJob);
				return;
			}
			if (xhr.status === 401) reportUnauthorized();
			const err = (body as Schemas['Error'] | null)?.error;
			reject(
				new ApiError(
					err?.message ?? `Upload failed (${xhr.status}).`,
					xhr.status,
					err?.code ??
						(xhr.status === 413 ? 'too_large' : xhr.status === 401 ? 'unauthorized' : undefined)
				)
			);
		};
		// A server that refuses a body part-way (413 before reading it all)
		// often surfaces as a network error rather than a response.
		xhr.onerror = () => reject(new ApiError('The upload did not complete.', 0, 'network'));
		xhr.onabort = () => reject(new ApiError('Upload cancelled.', 0, 'aborted'));
		xhr.send(file);
	});
	return { done, abort: () => xhr.abort() };
}

// ---------- what went wrong, and what to do ----------

export type ImportProblem = {
	title: string;
	body: string;
	/** Point at Spotify's privacy page (wrong export requested). */
	privacyLink?: boolean;
	/** Suggest `spotussi import <file>` on the server. */
	cli?: boolean;
};

const PROBLEMS: Record<string, ImportProblem> = {
	basic_export: {
		title: 'This is the basic Account data export',
		body: 'It has only the last year and no track IDs. Request Extended streaming history instead, a separate option in your Spotify privacy settings. Both downloads are called my_spotify_data.zip; check that the email says Extended streaming history.',
		privacyLink: true
	},
	no_history_files: {
		title: 'There’s no listening history in this file',
		body: 'The Extended streaming history zip holds files named Streaming_History_Audio_ followed by years. Check this is that export and not another Spotify download.',
		privacyLink: true
	},
	malformed_file: {
		title: 'Part of this file couldn’t be read',
		body: 'The zip or a JSON file in it is damaged, usually by an unfinished download. Download it again from Spotify’s email and upload it; streams already read are kept and skipped.'
	},
	unsupported_file: {
		title: 'That isn’t a zip or JSON file',
		body: 'Upload my_spotify_data.zip as Spotify sent it, or the Streaming_History_Audio JSON files from inside it.'
	},
	interrupted: {
		title: 'The server stopped during the import',
		body: 'Upload the same file again to finish. Streams already imported are skipped, so nothing counts twice.'
	},
	internal: {
		title: 'The import stopped on a server error',
		body: 'The server log has the details. Uploading the same file again is safe: streams already imported are skipped.'
	},
	too_large: {
		title: 'This file is larger than the server accepts',
		body: 'The limit is set with IMPORT_MAX_BYTES on the server. Raise it, or import the file on the server itself:',
		cli: true
	},
	bad_request: {
		title: 'The file is empty',
		body: 'Choose the export again. If it’s still empty, download it again from Spotify’s email.'
	},
	upload_failed: {
		title: 'The upload didn’t finish',
		body: 'The connection dropped before the whole file arrived, so nothing was imported. Try again, or for a very large file import it on the server:',
		cli: true
	},
	network: {
		title: 'The upload didn’t finish',
		body: 'The connection to the server dropped, or the server refused a file this large. Nothing was imported. Try again, or import the file on the server:',
		cli: true
	},
	unauthorized: {
		title: 'You’re signed out',
		body: 'Sign in again, then upload the file.'
	},
	read_only_token: {
		title: 'API tokens can’t import',
		body: 'Sign in with the owner password in this browser to upload an export.'
	},
	lost_contact: {
		title: 'Lost contact with the server',
		body: 'The import may still be running. Recent imports in Settings shows how it ends.'
	}
};

/** Copy for an upload error or a failed job's error_code. */
export function importProblem(code: string | null | undefined, message?: string | null) {
	return (
		(code && PROBLEMS[code]) || {
			title: 'The import didn’t finish',
			body: message ? `The server said: ${message}.` : 'Try uploading the file again.'
		}
	);
}
