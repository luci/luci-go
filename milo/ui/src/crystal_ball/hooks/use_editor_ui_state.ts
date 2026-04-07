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

import { useContext, useState, useCallback } from 'react';

import { EditorUiContext } from '@/crystal_ball/context';

/**
 * Allowed prefixes for Editor UI state keys.
 * Enforces structure and avoids magic strings.
 */
export enum EditorUiKeyPrefix {
  BREAKDOWN_SERIES = 'breakdown_series',
  CHART_SERIES = 'chart_series',
  GLOBAL_FILTERS = 'global_filters',
  WIDGET_FILTERS = 'widget_filters',
}

/**
 * Options for the useEditorUiState hook.
 */
export interface UseEditorUiStateOptions {
  /**
   * The prefix for the state key.
   * If undefined, the state will be local to the component and not persisted.
   */
  prefix?: EditorUiKeyPrefix;
  /**
   * The specific identifier for the state (e.g., widgetId, series name).
   * If undefined, only the prefix will be used as the key.
   */
  key?: string;
  /**
   * The initial value of the state if not already set in the context.
   * @default false
   */
  initialValue?: boolean;
}

/**
 * A hook to manage ephemeral UI state (like accordion expansion) that persists
 * across data refreshes and navigation, but resets on hard reload.
 *
 * If no prefix is provided, it falls back to standard local component state.
 */
export function useEditorUiState({
  prefix,
  key,
  initialValue = false,
}: UseEditorUiStateOptions): [boolean, (value: boolean) => void] {
  const context = useContext(EditorUiContext);
  const [localState, setLocalState] = useState(initialValue);

  if (!context) {
    throw new Error('useEditorUiState must be used within an EditorUiProvider');
  }

  const { uiStates, setUiState } = context;

  const setValue = useCallback(
    (val: boolean) => {
      if (prefix) {
        const fullKey = key ? `${prefix}:${key}` : prefix;
        setUiState(fullKey, val);
      }
    },
    [prefix, key, setUiState],
  );

  if (!prefix) {
    return [localState, setLocalState];
  }

  const fullKey = key ? `${prefix}:${key}` : prefix;
  const value = uiStates[fullKey] ?? initialValue;

  return [value, setValue];
}
