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

import { useState, ReactNode, useCallback, useMemo } from 'react';

import { EditorUiContext } from './editor_ui_context';

/**
 * Provider component for the EditorUiContext.
 * Wraps the application or component tree to provide shared ephemeral UI state.
 */
export function EditorUiProvider({ children }: { children: ReactNode }) {
  const [uiStates, setUiStates] = useState<Record<string, boolean>>({});

  const setUiState = useCallback((key: string, value: boolean) => {
    setUiStates((prev) => ({
      ...prev,
      [key]: value,
    }));
  }, []);

  const contextValue = useMemo(
    () => ({ uiStates, setUiState }),
    [uiStates, setUiState],
  );

  return (
    <EditorUiContext.Provider value={contextValue}>
      {children}
    </EditorUiContext.Provider>
  );
}
