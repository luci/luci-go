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

import { useId, useRef } from 'react';
import { useDeepCompareEffect } from 'react-use';

import { useShortcutContext } from './context';

export interface UseShortcutOptions {
  enabled?: boolean;
  category?: string;
}

/**
 * Registers a keyboard shortcut within the Fleet Console.
 *
 * @param description A human-readable description shown in the shortcuts help modal (triggered by '?').
 * @param keys The shortcut key definition. Can be a single string or an array of strings for alternatives.
 *             - Use '+' for modifiers (e.g., 'Control+s', 'Shift+Enter').
 *             - Use spaces for sequences (e.g., 'g h' means press 'g' then 'h').
 *             - Modifiers can be: 'Ctrl', 'Control', 'Meta', 'Cmd', 'Command', 'Alt', 'Option', 'Shift'.
 *             - Common key names are normalized: 'Esc', 'Enter', 'Space',
 *               'Tab', 'Backspace', 'Del', 'Delete', 'Up', 'Down', 'Left',
 *               'Right'.
 *             - Example sequences: 'g d' (navigate to devices),
 *               'Control+Shift+p' (command palette style).
 * @param handler The function to execute when the shortcut is triggered. Receives the raw KeyboardEvent.
 * @param options Configuration options.
 *                - `enabled`: If false, the shortcut will not be registered. Useful for conditional shortcuts.
 *                - `category`: The category under which the shortcut will be listed in the help modal. Defaults to 'General'.
 */
export const useShortcut = (
  description: string,
  keys: string | string[],
  handler: (e: KeyboardEvent) => void,
  options: UseShortcutOptions = {},
) => {
  const { register, unregister } = useShortcutContext();
  const id = useId();
  const { enabled = true, category = 'General' } = options;

  const handlerRef = useRef(handler);
  handlerRef.current = handler;

  useDeepCompareEffect(() => {
    if (!enabled) {
      return;
    }

    const safeHandler = (e: KeyboardEvent) => {
      handlerRef.current(e);
    };

    register({
      id,
      description,
      keys,
      handler: safeHandler,
      category,
    });

    return () => {
      unregister(id);
    };
  }, [id, description, keys, register, unregister, enabled, category]);
};
