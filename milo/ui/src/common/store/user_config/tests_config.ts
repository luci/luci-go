// Copyright 2023 The LUCI Authors.
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

import { types } from 'mobx-state-tree';

export const TestsConfig = types
  .model('TestsConfig', {
    /**
     * SHOULD NOT BE ACCESSED DIRECTLY. Use the methods instead.
     */
    // Map<ColumnKey, ColumnWidthInPx>
    _columnWidths: types.optional(types.map(types.number), {
      'v.test_suite': 350,
    }),
  })
  .views((self) => ({
    get columnWidths() {
      return Object.fromEntries(self._columnWidths.entries());
    },
  }))
  .actions((self) => ({
    setColumWidth(key: string, width: number) {
      self._columnWidths.set(key, width);
    },
  }));
