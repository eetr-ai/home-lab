"use client";

import { useSyncExternalStore } from "react";

/**
 * The log panel's remembered height.
 *
 * Read through useSyncExternalStore rather than in an effect, following the
 * theme switcher: the server snapshot is the default, the client snapshot is what
 * storage holds, and React reconciles the two without a synchronous setState in
 * an effect — which would be both a cascading render and a lint failure.
 */

export const MIN_HEIGHT = 120;
export const MAX_HEIGHT = 480;
export const DEFAULT_HEIGHT = 260;

const STORAGE_KEY = "home-lab.podlogs.height";

/** Same-tab notification. `storage` events only reach *other* tabs. */
const CHANGED_EVENT = "home-lab:podlogs-height-changed";

/**
 * Held in memory when storage is unavailable, so a browser blocking site data
 * still keeps the height for this page view instead of snapping back on every
 * drag.
 */
let inMemoryHeight: number | null = null;

function clamp(height: number): number {
	return Math.min(MAX_HEIGHT, Math.max(MIN_HEIGHT, height));
}

export function readPanelHeight(): number {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		const stored = raw === null ? Number.NaN : Number(raw);
		if (Number.isFinite(stored)) return clamp(stored);
	} catch {
		// Storage can throw outright when the browser blocks site data.
	}
	return inMemoryHeight ?? DEFAULT_HEIGHT;
}

export function writePanelHeight(height: number): void {
	inMemoryHeight = clamp(height);
	try {
		localStorage.setItem(STORAGE_KEY, String(inMemoryHeight));
	} catch {
		// A remembered height is a convenience, not something worth an error.
	}
	window.dispatchEvent(new Event(CHANGED_EVENT));
}

function subscribe(onChange: () => void): () => void {
	window.addEventListener("storage", onChange);
	window.addEventListener(CHANGED_EVENT, onChange);
	return () => {
		window.removeEventListener("storage", onChange);
		window.removeEventListener(CHANGED_EVENT, onChange);
	};
}

/** The stored height, or the default during the server render. */
export function usePanelHeight(): number {
	return useSyncExternalStore(subscribe, readPanelHeight, () => DEFAULT_HEIGHT);
}
