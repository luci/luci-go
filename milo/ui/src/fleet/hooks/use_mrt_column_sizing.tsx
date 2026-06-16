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

import { MRT_ColumnSizingState } from 'material-react-table';
import { useCallback, useEffect, useRef, useState } from 'react';

import { logging } from '@/common/tools/logging';

export function useMrtColumnSizing(localStorageKey: string): {
  columnSizing: MRT_ColumnSizingState;
  onColumnSizingChange: React.Dispatch<
    React.SetStateAction<MRT_ColumnSizingState>
  >;
  resetColumnWidths: () => void;
} {
  const currentKeyRef = useRef(localStorageKey);
  const isFirstRenderRef = useRef(true);
  const [columnSizing, setColumnSizing] = useState<MRT_ColumnSizingState>(
    () => {
      try {
        const stored = localStorage.getItem(`${localStorageKey}-sizes`);
        const parsed = stored ? JSON.parse(stored) : null;
        return parsed || {};
      } catch (e) {
        logging.error('Failed to load column sizing from localStorage', e);
        return {};
      }
    },
  );

  // Sync state with localStorage, loading the new configuration when the key changes
  // and saving the updated layout with a 500ms debounce when columns are resized.
  useEffect(() => {
    if (currentKeyRef.current !== localStorageKey) {
      try {
        const stored = localStorage.getItem(`${localStorageKey}-sizes`);
        const parsed = stored ? JSON.parse(stored) : null;
        setColumnSizing(parsed || {});
      } catch (e) {
        logging.error('Failed to load column sizing from localStorage', e);
        setColumnSizing({});
      }
      currentKeyRef.current = localStorageKey;
      return;
    }

    if (isFirstRenderRef.current) {
      isFirstRenderRef.current = false;
      return;
    }

    const handler = setTimeout(() => {
      try {
        localStorage.setItem(
          `${localStorageKey}-sizes`,
          JSON.stringify(columnSizing),
        );
      } catch (e) {
        logging.error('Failed to save column sizing to localStorage', e);
      }
    }, 500);

    return () => {
      clearTimeout(handler);
    };
  }, [columnSizing, localStorageKey]);

  const resetColumnWidths = useCallback(() => {
    try {
      localStorage.removeItem(`${localStorageKey}-sizes`);
    } catch (e) {
      logging.error('Failed to remove column sizing from localStorage', e);
    }
    setColumnSizing({});
  }, [localStorageKey]);

  return {
    columnSizing,
    onColumnSizingChange: setColumnSizing,
    resetColumnWidths,
  };
}
