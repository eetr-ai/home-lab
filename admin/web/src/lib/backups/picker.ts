import type { BackupFolder } from "@/lib/api/types";

/**
 * Google's folder Picker.
 *
 * The fourth third-party UI exception in docs/contributing/ux-guidelines.md, and
 * the only one loaded from Google rather than bundled: the Picker is how
 * drive.file reaches a folder the panel did not create. Picking a folder is what
 * grants the panel access to it — there is no way to offer that choice with the
 * narrow scope short of Google's own dialog.
 *
 * The script is loaded on first use, never on page load, so a lab that never
 * opens the picker never fetches anything from Google.
 */

const SCRIPT = "https://apis.google.com/js/api.js";

/** The slice of the Picker's API this module uses. */
interface PickerBuilder {
	addView(view: unknown): PickerBuilder;
	setOAuthToken(token: string): PickerBuilder;
	setDeveloperKey(key: string): PickerBuilder;
	setAppId(id: string): PickerBuilder;
	setTitle(title: string): PickerBuilder;
	setCallback(callback: (data: unknown) => void): PickerBuilder;
	build(): { setVisible(visible: boolean): void };
}

interface PickerNamespace {
	DocsView: new (viewId: string) => {
		setIncludeFolders(value: boolean): unknown;
		setSelectFolderEnabled(value: boolean): unknown;
		setMimeTypes(types: string): unknown;
	};
	PickerBuilder: new () => PickerBuilder;
	ViewId: { FOLDERS: string };
}

declare global {
	interface Window {
		gapi?: { load(name: string, callback: () => void): void };
		google?: { picker?: PickerNamespace };
	}
}

const FOLDER_MIME = "application/vnd.google-apps.folder";

let loading: Promise<PickerNamespace> | null = null;

function loadPicker(): Promise<PickerNamespace> {
	if (window.google?.picker) return Promise.resolve(window.google.picker);
	loading ??= new Promise<PickerNamespace>((resolve, reject) => {
		const script = document.createElement("script");
		script.src = SCRIPT;
		script.async = true;
		script.onerror = () => {
			loading = null;
			reject(new Error("Google's picker could not be loaded"));
		};
		script.onload = () =>
			window.gapi?.load("picker", () => {
				if (window.google?.picker) resolve(window.google.picker);
				else reject(new Error("Google's picker loaded without a picker in it"));
			});
		document.head.appendChild(script);
	});
	return loading;
}

/**
 * Open the Picker on folders, and resolve with the one chosen, or null when the
 * operator closed it without choosing.
 */
export async function pickFolder(options: {
	accessToken: string;
	apiKey: string;
	appId: string;
}): Promise<BackupFolder | null> {
	const picker = await loadPicker();
	const view = new picker.DocsView(picker.ViewId.FOLDERS);
	view.setIncludeFolders(true);
	view.setSelectFolderEnabled(true);
	view.setMimeTypes(FOLDER_MIME);

	return new Promise((resolve) => {
		new picker.PickerBuilder()
			.addView(view)
			.setOAuthToken(options.accessToken)
			.setDeveloperKey(options.apiKey)
			.setAppId(options.appId)
			.setTitle("Choose the folder backups go into")
			.setCallback((data) => {
				const outcome = readPickerResult(data);
				if (outcome !== "pending") resolve(outcome);
			})
			.build()
			.setVisible(true);
	});
}

/**
 * Read what the Picker reported.
 *
 * "pending" for the callbacks that are not an answer — the Picker also calls back
 * when it loads — a folder for a pick, null for a cancel. Shape-checked, because
 * it is a third party's object arriving in a callback, and a pick that is not a
 * folder must not become a folder id.
 */
export function readPickerResult(data: unknown): BackupFolder | null | "pending" {
	const record = data as { action?: string; docs?: { id?: unknown; name?: unknown; mimeType?: unknown }[] };
	if (record?.action === "cancel") return null;
	if (record?.action !== "picked") return "pending";

	const doc = record.docs?.[0];
	if (!doc || typeof doc.id !== "string" || doc.mimeType !== FOLDER_MIME) return null;
	return { id: doc.id, name: typeof doc.name === "string" ? doc.name : doc.id };
}
