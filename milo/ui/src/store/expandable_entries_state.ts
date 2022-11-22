// Copyright 2022 The LUCI Authors.
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

import { Instance, types } from 'mobx-state-tree';

export const ExpandableEntriesState = types
  .model('ExpandableEntriesState', {
    defaultExpanded: false,
    _entryExpanded: types.optional(
      types.map(
        types.model({
          id: types.identifier,
          expanded: types.boolean,
        })
      ),
      {}
    ),
  })
  .views((self) => ({
    /**
     * Whether the entry is expanded.
     */
    isExpanded(id: string) {
      return self._entryExpanded.get(id)?.expanded ?? self.defaultExpanded;
    },
  }))
  .actions((self) => ({
    /**
     * Toggle all entries.
     */
    toggleAll(expand: boolean) {
      self.defaultExpanded = expand;
      self._entryExpanded.clear();
    },
    /**
     * Toggle a single entry.
     */
    toggle(id: string, expand: boolean) {
      self._entryExpanded.put({ id, expanded: expand });
    },
  }));

export type ExpandableEntriesStateInstance = Instance<typeof ExpandableEntriesState>;
