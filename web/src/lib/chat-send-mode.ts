"use client";

import * as React from "react";

import { getLocalStorageValue, setLocalStorageValue } from "@/lib/local-storage.client";

// Sending session input box/Line feed key. Pure front-end preference: only drop localStorage,Not stored in the database, not synchronized with the account,
// So changing browsers requires resetting. see issue #39——0.3.2 handle Ctrl+Enter Send changed to Enter Send,
// Returning the old keys is optional here.
export type ChatSendMode = "enter" | "ctrl-enter";

export const CHAT_SEND_MODE_KEY = "artex_chat_send_mode";
export const DEFAULT_CHAT_SEND_MODE: ChatSendMode = "enter";

export const CHAT_SEND_MODE_OPTIONS: { value: ChatSendMode; label: string }[] = [
  { value: "enter", label: "Enter to send, Shift+Enter to wrap" },
  { value: "ctrl-enter", label: "Ctrl+Enter to send, Enter to wrap" },
];

function parseMode(raw: string | null): ChatSendMode {
  return raw === "ctrl-enter" || raw === "enter" ? raw : DEFAULT_CHAT_SEND_MODE;
}

// Subscriber collection within the same tab.localStorage of storage Events only in[Others]Tab trigger,
// This page needs to be changed after changing it in the settings. emit Notify the input box on the same page, otherwise it must be refreshed to take effect.
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  window.addEventListener("storage", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", listener);
  };
}

// Returns a string literal,Object.is Compare by value, will not let useSyncExternalStore Stuck in a loop.
function getSnapshot(): ChatSendMode {
  return parseMode(getLocalStorageValue(CHAT_SEND_MODE_KEY));
}

// Not available on the server localStorage,Render the default value first,hydrate After getSnapshot Correct again.
function getServerSnapshot(): ChatSendMode {
  return DEFAULT_CHAT_SEND_MODE;
}

export function useChatSendMode(): ChatSendMode {
  return React.useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function setChatSendMode(mode: ChatSendMode) {
  setLocalStorageValue(CHAT_SEND_MODE_KEY, mode);
  for (const listener of listeners) listener();
}

// shouldSubmitOnKey Determine whether a keystroke should be sent.
// isComposing / keyCode 229 The input method such as Chinese is selecting words and must be released, otherwise the word selection by pressing Enter will be sent by mistake..
// enter Pattern exclude only Shift,With 0.3.2 The behavior of——For users who do not change the settings, the feel remains unchanged..
// ctrl-enter Mode accept simultaneously Ctrl With Cmd(macOS).
export function shouldSubmitOnKey(e: React.KeyboardEvent, mode: ChatSendMode): boolean {
  if (e.key !== "Enter") return false;
  if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return false;
  if (mode === "ctrl-enter") return e.ctrlKey || e.metaKey;
  return !e.shiftKey;
}
