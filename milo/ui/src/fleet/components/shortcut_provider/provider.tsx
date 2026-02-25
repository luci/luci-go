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

import { useState, useEffect, useCallback, useMemo, useRef } from 'react';

import { isTyping } from '@/fleet/utils/field_typing';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import { ShortcutContext, ShortcutDefinition } from './context';
import { ShortcutModal } from './shortcut_modal';
import { ShortcutsTip } from './shortcuts_tip';
import { parseShortcut, KeyChord, ShortcutSequence } from './utils';

interface ParsedShortcut extends ShortcutDefinition {
  sequences: ShortcutSequence[];
}

const isModifierKey = (key: string) =>
  ['Control', 'Shift', 'Alt', 'Meta'].includes(key);

const chordsMatch = (b: KeyChord, t: KeyChord) => {
  if (b.key.toLowerCase() !== t.key.toLowerCase()) return false;
  if (b.ctrl !== t.ctrl) return false;
  if (b.meta !== t.meta) return false;
  if (b.alt !== t.alt) return false;

  if (b.shift !== t.shift) {
    // Allow implicit shift for characters (e.g. '?' or 'A') where the key itself implies shift
    const isImplicitShift =
      b.shift && !t.shift && b.key.length === 1 && b.key === t.key;
    if (!isImplicitShift) return false;
  }
  return true;
};

const sequencesMatch = (a: KeyChord[], b: KeyChord[]) => {
  if (a.length > b.length) return false;
  return a.every((chord, i) => chordsMatch(chord, b[i]));
};

const checkConflicts = (
  newSeq: ShortcutSequence,
  newDef: ShortcutDefinition,
  existing: ParsedShortcut[],
) => {
  existing.forEach((s) => {
    s.sequences.forEach((existingSeq) => {
      // Check if one is a prefix of the other (including exact match)
      const newIsPrefix = sequencesMatch(newSeq, existingSeq);
      const existingIsPrefix = sequencesMatch(existingSeq, newSeq);

      if (newIsPrefix || existingIsPrefix) {
        const type =
          newSeq.length === existingSeq.length
            ? 'Exact conflict'
            : 'Prefix conflict';

        throw new Error(
          `Shortcut conflict detected (${type}):\n` +
            `  New: "${newDef.keys}" (${newDef.description})\n` +
            `  Existing: "${s.keys}" (${s.description})`,
        );
      }
    });
  });
};

export const ShortcutProvider = ({
  children,
}: {
  children: React.ReactNode;
}) => {
  const [shortcuts, setShortcuts] = useState<ParsedShortcut[]>([]);
  const bufferRef = useRef<KeyChord[]>([]);
  const { trackEvent } = useGoogleAnalytics();

  const register = useCallback((def: ShortcutDefinition) => {
    const keyList = Array.isArray(def.keys) ? def.keys : [def.keys];
    const sequences = keyList.map(parseShortcut);

    setShortcuts((prev) => {
      // Check for conflicts with existing shortcuts (excluding self if updating)
      const others = prev.filter((s) => s.id !== def.id);
      sequences.forEach((seq) => checkConflicts(seq, def, others));

      return [...others, { ...def, sequences }];
    });
  }, []);

  const unregister = useCallback((id: string) => {
    setShortcuts((prev) => prev.filter((s) => s.id !== id));
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.defaultPrevented || isModifierKey(e.key)) return;

      const chord: KeyChord = {
        key: e.key,
        ctrl: e.ctrlKey,
        meta: e.metaKey,
        alt: e.altKey,
        shift: e.shiftKey,
      };

      if (
        isTyping(e.target as Element) &&
        !chord.ctrl &&
        !chord.alt &&
        !chord.meta
      ) {
        bufferRef.current = [];
        return;
      }

      const execute = (s: ParsedShortcut) => {
        trackEvent('shortcut_triggered', {
          componentName: s.description,
        });
        s.handler(e);
        e.preventDefault();
        bufferRef.current = [];
      };

      const buffer = (newBuffer: KeyChord[]) => {
        e.preventDefault();
        bufferRef.current = newBuffer;
      };

      const findMatches = (buf: KeyChord[]) =>
        shortcuts.filter((s) =>
          s.sequences.some(
            (seq) =>
              seq.length >= buf.length &&
              buf.every((c, i) => chordsMatch(c, seq[i])),
          ),
        );

      const findExact = (candidates: ParsedShortcut[], length: number) =>
        candidates.find((s) =>
          s.sequences.some((seq) => seq.length === length),
        );

      // 1. Try extending the current buffer
      const nextBuffer = [...bufferRef.current, chord];
      const ongoingMatches = findMatches(nextBuffer);

      if (ongoingMatches.length > 0) {
        const exact = findExact(ongoingMatches, nextBuffer.length);
        if (exact) {
          execute(exact);
        } else {
          buffer(nextBuffer);
        }
        return;
      }

      // 2. Try starting a new sequence
      const startMatches = findMatches([chord]);
      if (startMatches.length > 0) {
        const exact = findExact(startMatches, 1);
        if (exact) {
          execute(exact);
        } else {
          buffer([chord]);
        }
        return;
      }

      // 3. No match
      bufferRef.current = [];
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [shortcuts, trackEvent]);

  const value = useMemo(
    () => ({
      register,
      unregister,
    }),
    [register, unregister],
  );

  return (
    <ShortcutContext.Provider value={value}>
      {children}
      <ShortcutsTip />
      <ShortcutModal shortcuts={shortcuts} />
    </ShortcutContext.Provider>
  );
};
