"use server";

import { cookies } from "next/headers";
import { revalidatePath } from "next/cache";

import * as backups from "@/lib/api/backups";
import {
	authorizationUrl,
	CALLBACK_PATH,
	challengeFor,
	FLOW_COOKIE,
	FLOW_TTL_SECONDS,
	newFlow,
	redirectUriFor,
	sealFlow,
} from "@/lib/backups/oauth";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	BackupFile,
	BackupRun,
	BackupSchedule,
	BackupScheduleRequest,
	GoogleAccessToken,
	GoogleStatus,
} from "@/lib/api/types";

/**
 * The Backups section's actions.
 *
 * Connecting Drive starts here and finishes in the callback route handler,
 * because Google sends the browser back to a URL, which an action cannot be.
 * Everything else is the usual shape: read with withRead, write with withWrite,
 * and revalidate the whole section after a write.
 */
const SECTION = "/backups";

export async function googleStatus(): Promise<ActionResult<GoogleStatus>> {
	return withRead(() => backups.googleStatus());
}

/**
 * Start connecting a Google account: answer with the consent URL, and leave the
 * flow's state and PKCE verifier in a sealed, short-lived cookie for the callback
 * to check. The browser navigates; nothing is exchanged here.
 */
export async function beginGoogleConnect(): Promise<ActionResult<{ url: string }>> {
	return withWrite(async () => {
		const status = await backups.googleStatus();
		if (!status.ok) return status;
		if (!status.data.configured || !status.data.clientId) {
			return { ok: false, error: "this lab has no Google OAuth client yet; see docs/backups.md" };
		}

		const redirectUri = redirectUriFor(process.env.AUTH_URL);
		const secret = process.env.AUTH_SECRET;
		if (!redirectUri || !secret) {
			return { ok: false, error: "AUTH_URL and AUTH_SECRET must be set to connect Google Drive" };
		}

		const flow = newFlow(Date.now());
		(await cookies()).set(FLOW_COOKIE, await sealFlow(flow, secret), {
			httpOnly: true,
			secure: redirectUri.startsWith("https:"),
			// Lax, not Strict: the callback is a top-level navigation arriving from
			// Google, and a Strict cookie would not be sent with it.
			sameSite: "lax",
			path: CALLBACK_PATH,
			maxAge: FLOW_TTL_SECONDS,
		});

		const url = authorizationUrl({
			clientId: status.data.clientId,
			redirectUri,
			scopes: status.data.scopes ?? [],
			state: flow.state,
			challenge: await challengeFor(flow.verifier),
		});
		return { ok: true, data: { url } };
	});
}

export async function disconnectGoogle(): Promise<ActionResult<void>> {
	const result = await withWrite(() => backups.disconnectGoogle());
	if (result.ok) revalidatePath(SECTION, "layout");
	return result;
}

/**
 * A token for the folder Picker. A write, because handing the browser a Drive
 * credential is something only an operator who may change things should get.
 */
export async function pickerToken(): Promise<ActionResult<GoogleAccessToken>> {
	return withWrite(() => backups.pickerToken());
}

export async function listSchedules(): Promise<ActionResult<BackupSchedule[]>> {
	return withRead(() => backups.listSchedules());
}

export async function readSchedule(name: string): Promise<ActionResult<BackupSchedule>> {
	return withRead(() => backups.readSchedule(name));
}

export async function createSchedule(
	request: BackupScheduleRequest,
): Promise<ActionResult<BackupSchedule>> {
	const result = await withWrite(() => backups.createSchedule(request));
	if (result.ok) revalidatePath(SECTION, "layout");
	return result;
}

export async function updateSchedule(
	name: string,
	request: BackupScheduleRequest,
): Promise<ActionResult<BackupSchedule>> {
	const result = await withWrite(() => backups.updateSchedule(name, request));
	if (result.ok) revalidatePath(SECTION, "layout");
	return result;
}

export async function deleteSchedule(name: string): Promise<ActionResult<void>> {
	const result = await withWrite(() => backups.deleteSchedule(name));
	if (result.ok) revalidatePath(SECTION, "layout");
	return result;
}

export async function runSchedule(name: string): Promise<ActionResult<BackupRun>> {
	const result = await withWrite(() => backups.runSchedule(name));
	if (result.ok) revalidatePath(SECTION, "layout");
	return result;
}

export async function listRuns(name: string): Promise<ActionResult<BackupRun[]>> {
	return withRead(() => backups.listRuns(name));
}

export async function listFiles(name: string): Promise<ActionResult<BackupFile[]>> {
	return withRead(() => backups.listFiles(name));
}
