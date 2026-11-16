import { useCallback, useEffect, useLayoutEffect, useRef } from 'react';
import { useAtomValue } from 'jotai';

import { getOperatingSystem } from '@/lib/browser.ts';
import { KeyboardReport } from '@/lib/keyboard.ts';
import { isModifier } from '@/lib/keymap';
import { client, MessageEvent } from '@/lib/websocket.ts';
import { isKeyboardEnableAtom } from '@/jotai/keyboard.ts';
import { picoclawTakeoverStateAtom } from '@/jotai/picoclaw.ts';

import { isPasteShortcut, usePaste } from '../paste/use-paste.ts';
import { isEditableTarget } from './editable-target.ts';
import { isCompositionKey, ModifierSync } from './input-state.ts';
import { Recorder } from './recorder.tsx';
import { useAltGr } from './useAltGr.ts';
import { useLeaderKey } from './useLeaderKey.ts';
import { normalizeKeyCode } from './utils.ts';

function sendReport(report: Uint8Array) {
  const data = new Uint8Array([MessageEvent.Keyboard, ...report]);
  client.send(data);
}

export const Keyboard = () => {
  const os = getOperatingSystem();

  const isKeyboardEnabled = useAtomValue(isKeyboardEnableAtom);
  const picoclawTakeoverState = useAtomValue(picoclawTakeoverStateAtom);

  const keyboardRef = useRef(new KeyboardReport());
  const pressedKeys = useRef(new Set<string>());
  const isComposing = useRef(false);
  const modifierSync = useRef(new ModifierSync());
  // Keys whose press started the paste shortcut. The host has been told they
  // are released, so their auto-repeat and their keyup stay on this side.
  const swallowedKeys = useRef(new Set<string>());

  // Send key event helper. It reads only refs, so one instance serves every
  // render, and the listener effect below can name it without re-subscribing.
  // A re-subscribe is not free: its cleanup releases every pressed key.
  const sendKeyEvent = useCallback((type: 'keydown' | 'keyup', code: string) => {
    const kb = keyboardRef.current;
    const report = type === 'keydown' ? kb.keyDown(code) : kb.keyUp(code);
    sendReport(report);
  }, []);

  // Init leader key handler
  const leaderKey = useLeaderKey(pressedKeys, sendKeyEvent);
  const leaderKeyRef = useRef(leaderKey);

  // Init AltGr key handler
  const altGr = useAltGr(os, pressedKeys, sendKeyEvent);
  const altGrRef = useRef(altGr);

  // Init paste shortcut handler
  const { pasteClipboard } = usePaste();
  const pasteClipboardRef = useRef(pasteClipboard);

  // The listeners read the handlers through these refs. They are brought up to
  // date after each commit and before any effect runs, which is before any
  // event can reach a listener.
  useLayoutEffect(() => {
    leaderKeyRef.current = leaderKey;
    altGrRef.current = altGr;
    pasteClipboardRef.current = pasteClipboard;
  });

  useEffect(() => {
    if (!picoclawTakeoverState.active) {
      return;
    }

    pressedKeys.current.clear();
    modifierSync.current.reset();
    leaderKeyRef.current.reset();
    altGrRef.current.reset();
    sendReport(keyboardRef.current.reset());
  }, [picoclawTakeoverState.active]);

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('keyup', handleKeyUp);
    document.addEventListener('compositionstart', handleCompositionStart);
    document.addEventListener('compositionend', handleCompositionEnd);
    document.addEventListener('focusin', handleFocusIn);
    document.addEventListener('pointerdown', handlePointerDown, true);
    window.addEventListener('blur', handleBlur);
    document.addEventListener('visibilitychange', handleVisibilityChange);

    function handleKeyDown(event: KeyboardEvent) {
      if (!isKeyboardEnabled) return;
      // A field of the page owns the keyboard while it has focus. Forwarding
      // the key would type it on the host as well, and cancelling its default
      // would keep it out of the field. This comes before the composition
      // check, so a key typed into a field never touches the composing flag.
      if (isEditableTarget(event.target)) return;
      if (isCompositionKey(isComposing.current, event)) return;
      // A keydown the input method is not consuming means any composition is
      // over, whether or not its compositionend reached the document.
      isComposing.current = false;

      event.preventDefault();
      event.stopPropagation();

      const code = normalizeKeyCode(event, os);
      if (!code) return;

      syncModifiers(event, 'keydown', code);

      if (pressedKeys.current.has(code) || swallowedKeys.current.has(code)) return;

      // Handle paste shortcut. Its modifiers have reached the host already;
      // they are released first, or the host would read the pasted text as
      // Ctrl+Alt+Shift chords.
      if (isPasteShortcut(event)) {
        const held = Array.from(pressedKeys.current);
        releaseKeys();
        swallowedKeys.current = new Set([...held, code]);
        pasteClipboardRef.current();
        return;
      }

      // Handle leader key
      const leaderHandled = leaderKeyRef.current.handleKeyDown(code);
      if (leaderHandled) {
        return;
      }

      // Handle AltGr key
      altGrRef.current.handleKeyDown(code, event.timeStamp);

      pressedKeys.current.add(code);
      sendKeyEvent('keydown', code);
    }

    function handleKeyUp(event: KeyboardEvent) {
      if (!isKeyboardEnabled) return;

      const code = normalizeKeyCode(event, os);
      if (!code) return;

      if (swallowedKeys.current.delete(code)) {
        event.preventDefault();
        event.stopPropagation();
        return;
      }

      // A key already reported as pressed is always released, whatever the IME
      // is doing. The guard used to skip every keyup while composing, which is
      // right for a key the IME is consuming and wrong for one this side has
      // already sent a keydown for. The JIS Zenkaku/Hankaku key is exactly
      // that case: it toggles the local IME, so compositionstart can arrive
      // between its own keydown and keyup. The keyup was dropped, the host was
      // left holding the key, and because this side still had it in
      // pressedKeys its next keydown was suppressed too. One press and the key
      // never worked again.
      const wasPressed = pressedKeys.current.has(code);

      // The keyup of a key typed into a field of the page stays with the
      // field. One the host still holds is released all the same.
      if (isEditableTarget(event.target) && !wasPressed) return;

      if ((isComposing.current || event.isComposing) && !wasPressed) return;

      event.preventDefault();
      event.stopPropagation();

      syncModifiers(event, 'keyup', code);
      releaseKey(code);
    }

    // The release half of handleKeyUp, also used for a modifier whose keyup
    // never came, so the leader key and AltGr see it as an ordinary release.
    function releaseKey(code: string) {
      // Handle leader key release
      const leaderHandled = leaderKeyRef.current.handleKeyUp(code);
      if (leaderHandled) {
        return;
      }

      // Handle AltGr key release
      const altGrHandled = altGrRef.current.handleKeyUp(code);
      if (altGrHandled) {
        return;
      }

      // Release Meta combinations (macOS compatibility)
      if (code === 'MetaLeft' || code === 'MetaRight') {
        releaseMetaCombinations();
      }

      pressedKeys.current.delete(code);
      sendKeyEvent('keyup', code);
    }

    // Focus moving into a field of the page takes the keyups with it: they
    // land in the field and are not forwarded. Whatever the host still holds
    // is released now, or a shortcut that opened a dialog would leave its
    // modifier down on the host for as long as the user types in the dialog.
    function handleFocusIn(event: FocusEvent) {
      if (!isKeyboardEnabled) return;
      if (isEditableTarget(event.target)) {
        releaseKeys();
      }
    }

    // A press on the host screen hands the keyboard back to the host. The
    // mouse handlers cancel the default of that press, so the browser does
    // not take the focus out of a field by itself, and the keys would keep
    // going to a field the user has left.
    function handlePointerDown(event: PointerEvent) {
      const screen = document.getElementById('screen');
      if (!screen || !(event.target instanceof Node) || !screen.contains(event.target)) return;

      const focused = document.activeElement;
      if (focused instanceof HTMLElement && isEditableTarget(focused)) {
        focused.blur();
      }
    }

    function handleCompositionStart() {
      isComposing.current = true;
    }

    function handleCompositionEnd() {
      isComposing.current = false;
    }

    // A modifier the host still holds but the browser reports as up lost its
    // keyup, to an OS shortcut or a dialog that took focus inside the page.
    // Release it now rather than leave it held until the next blur. A side that
    // went with its pair's keyup and is reported as held again is pressed again.
    function syncModifiers(event: KeyboardEvent, type: 'keydown' | 'keyup', code: string) {
      const changes = modifierSync.current.sync(pressedKeys.current, event, type, code);
      for (const modifier of changes.release) {
        releaseKey(modifier);
      }
      for (const modifier of changes.press) {
        pressedKeys.current.add(modifier);
        sendKeyEvent('keydown', modifier);
      }
    }

    function handleBlur() {
      releaseKeys();
    }

    function handleVisibilityChange() {
      if (document.hidden) {
        releaseKeys();
      }
    }

    function releaseMetaCombinations() {
      const keysToRelease = Array.from(pressedKeys.current).filter((code) => !isModifier(code));
      for (const key of keysToRelease) {
        sendKeyEvent('keyup', key);
        pressedKeys.current.delete(key);
      }
    }

    function releaseKeys() {
      pressedKeys.current.forEach((code) => {
        sendKeyEvent('keyup', code);
      });
      pressedKeys.current.clear();
      swallowedKeys.current.clear();
      modifierSync.current.reset();

      // Releasing everything ends any composition as far as this side is
      // concerned. Leaving the flag set would have the next keyup skipped for a
      // key that is no longer tracked as pressed, which is how the fault above
      // starts.
      isComposing.current = false;

      leaderKeyRef.current.reset();
      altGrRef.current.reset();

      const report = keyboardRef.current.reset();
      sendReport(report);
    }

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('keyup', handleKeyUp);
      document.removeEventListener('compositionstart', handleCompositionStart);
      document.removeEventListener('compositionend', handleCompositionEnd);
      document.removeEventListener('focusin', handleFocusIn);
      document.removeEventListener('pointerdown', handlePointerDown, true);
      window.removeEventListener('blur', handleBlur);
      document.removeEventListener('visibilitychange', handleVisibilityChange);

      releaseKeys();
    };
  }, [isKeyboardEnabled, os, sendKeyEvent]);

  return (
    <>
      <Recorder recordMode={leaderKey.recordMode} recordedKeys={leaderKey.recordedKeys} />
    </>
  );
};
