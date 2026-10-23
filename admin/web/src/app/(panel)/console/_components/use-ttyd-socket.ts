"use client";

import { useEffect, useRef, useState } from "react";
import {
	TTYD_PROTOCOL,
	decodeServerMessage,
	initMessage,
	inputMessage,
	resizeMessage,
	socketUrl,
} from "@/lib/console/ttyd";
import type { Xterm } from "./use-xterm";

/**
 * Where the connection stands. `refused` is a socket that never opened — the gate
 * said no, or the console is not running — which reads differently from a
 * connection that was open and dropped.
 */
export type SocketStatus = "connecting" | "open" | "dropped" | "refused";

/** ttyd closes cleanly when the program it ran exits: here, tmux detaching or ending. */
const NORMAL_CLOSURE = 1000;

/**
 * Connect `xterm` to a tmux session through the console gate.
 *
 * A clean close from an open socket means the shell went away on purpose —
 * `exit`, or a tmux detach — and calls `onEnded`, which closes the tab. Anything
 * else leaves the tab up with a reconnect, because the session is very likely
 * still running and reconnecting is all it takes to get back to it.
 */
export function useTtydSocket(
	xterm: Xterm | null,
	name: string,
	socketBase: string,
	onEnded: () => void,
): { status: SocketStatus; reconnect: () => void } {
	const [status, setStatus] = useState<SocketStatus>("connecting");
	const [attempt, setAttempt] = useState(0);

	// The latest callback without making it a reason to reconnect.
	const ended = useRef(onEnded);
	useEffect(() => {
		ended.current = onEnded;
	}, [onEnded]);

	useEffect(() => {
		if (!xterm) return;
		const { term, fit } = xterm;
		const socket = new WebSocket(socketUrl(socketBase, name, window.location), TTYD_PROTOCOL);
		socket.binaryType = "arraybuffer";
		let opened = false;

		const send = (message: string | Uint8Array) => {
			if (socket.readyState === WebSocket.OPEN) socket.send(message);
		};

		socket.onopen = () => {
			opened = true;
			setStatus("open");
			socket.send(initMessage({ columns: term.cols, rows: term.rows }));
			term.focus();
		};
		socket.onmessage = (event: MessageEvent<ArrayBuffer>) => {
			const message = decodeServerMessage(event.data);
			if (message.kind === "output") term.write(message.data);
		};
		socket.onclose = (event) => {
			if (opened && event.code === NORMAL_CLOSURE) {
				ended.current();
				return;
			}
			setStatus(opened ? "dropped" : "refused");
		};

		const typing = term.onData((data) => send(inputMessage(data)));
		const resizing = term.onResize(({ cols, rows }) => send(resizeMessage({ columns: cols, rows })));
		fit.fit();

		return () => {
			typing.dispose();
			resizing.dispose();
			// Our own close is not the shell ending; do not let it close the tab.
			socket.onclose = null;
			socket.close();
		};
	}, [xterm, name, socketBase, attempt]);

	const reconnect = () => {
		setStatus("connecting");
		xterm?.term.reset();
		setAttempt((n) => n + 1);
	};

	return { status, reconnect };
}
