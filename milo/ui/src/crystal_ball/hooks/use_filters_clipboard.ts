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

import { useLocalStorage } from 'react-use';

import { PerfFilter } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

const FILTERS_CLIPBOARD_KEY = 'crystal_ball_filters_clipboard';

/**
 * A custom hook to manage the filters clipboard using localStorage.
 * Supports cross-tab and same-tab reactivity via react-use.
 */
export function useFiltersClipboard() {
  const [filters = [], setFilters, clearFilters] = useLocalStorage<
    PerfFilter[]
  >(FILTERS_CLIPBOARD_KEY, []);

  const clipboardCount = filters.length;

  const copyFilters = (newFilters: PerfFilter[]) => {
    setFilters(newFilters);
  };

  const getClipboardFilters = () => filters;

  const clearClipboard = () => {
    clearFilters();
  };

  return {
    clipboardCount,
    copyFilters,
    getClipboardFilters,
    clearClipboard,
  };
}
