// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

export interface KeyChord {
  key: string;
  ctrl: boolean;
  meta: boolean;
  alt: boolean;
  shift: boolean;
}

export type ShortcutSequence = KeyChord[];

/**
 * Normalizes a key string to match KeyboardEvent.key values.
 * e.g. "esc" -> "Escape"
 */
function normalizeKey(key: string): string {
  const lower = key.toLowerCase();
  switch (lower) {
    case 'esc':
    case 'escape':
      return 'Escape';
    case 'enter':
    case 'return':
      return 'Enter';
    case 'space':
    case ' ':
      return ' ';
    case 'tab':
      return 'Tab';
    case 'backspace':
      return 'Backspace';
    case 'delete':
    case 'del':
      return 'Delete';
    case 'arrowup':
    case 'up':
      return 'ArrowUp';
    case 'arrowdown':
    case 'down':
      return 'ArrowDown';
    case 'arrowleft':
    case 'left':
      return 'ArrowLeft';
    case 'arrowright':
    case 'right':
      return 'ArrowRight';
    default:
      return key;
  }
}

export function parseShortcut(input: string): ShortcutSequence {
  // Split sequences by '>' or space.
  // react-hotkeys-hook uses '>' for sequences.
  const parts = input.trim().split(/\s*>\s*|\s+/);

  return parts.map((part) => {
    const keys = part.split('+');
    const chord: KeyChord = {
      key: '',
      ctrl: false,
      meta: false,
      alt: false,
      shift: false,
    };

    keys.forEach((k) => {
      const lower = k.toLowerCase();
      if (lower === 'ctrl' || lower === 'control') chord.ctrl = true;
      else if (lower === 'meta' || lower === 'cmd' || lower === 'command')
        chord.meta = true;
      else if (lower === 'alt' || lower === 'option') chord.alt = true;
      else if (lower === 'shift') chord.shift = true;
      else chord.key = normalizeKey(k);
    });

    // If key is uppercase and length 1, and shift wasn't explicitly set,
    // we might want to consider how to handle it.
    // But usually for shortcuts, "Shift+s" is preferred over "S".
    // Let's stick to what the user provided.

    return chord;
  });
}

export function matchesChord(event: KeyboardEvent, chord: KeyChord): boolean {
  // We ignore modifiers if the chord key is a modifier itself (e.g. user defined shortcut as just "Control")
  // But that's rare.

  if (chord.key.toLowerCase() !== event.key.toLowerCase()) {
    return false;
  }

  // Strict modifier matching
  if (event.ctrlKey !== chord.ctrl) return false;
  if (event.metaKey !== chord.meta) return false;
  if (event.altKey !== chord.alt) return false;
  if (event.shiftKey !== chord.shift) return false;

  return true;
}

export function formatShortcut(sequence: ShortcutSequence): string[][] {
  return sequence.map((chord) => {
    const parts = [];
    if (chord.meta) parts.push('Meta');
    if (chord.ctrl) parts.push('Ctrl');
    if (chord.alt) parts.push('Alt');
    if (chord.shift) parts.push('Shift');

    let key = chord.key;
    if (key === ' ') key = 'Space';
    parts.push(key);

    return parts;
  });
}
