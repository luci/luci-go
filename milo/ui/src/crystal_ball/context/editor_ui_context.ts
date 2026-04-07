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

import { createContext } from 'react';

/**
 * The shape of the EditorUiContext.
 */
export interface EditorUiContextType {
  /**
   * A map of unique keys to their corresponding boolean states (e.g., expanded state).
   */
  uiStates: Record<string, boolean>;
  /**
   * A function to update the state for a specific key.
   */
  setUiState: (key: string, value: boolean) => void;
}

/**
 * React Context for managing ephemeral UI states across the Crystal Ball dashboard.
 * Should be used via the `useEditorUiState` hook.
 */
export const EditorUiContext = createContext<EditorUiContextType | undefined>(
  undefined,
);
