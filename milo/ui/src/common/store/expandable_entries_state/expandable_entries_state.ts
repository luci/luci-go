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

/**
 * A MobX state that manages expandable entires state. Comparing to a regular
 * combination of React context and React state, this helps reduce rerendering
 * when the default state is changed.
 *
 * If we store the default expanded state as a regular React state,
 *  * all non-memo-wrapped child components of the table will be rerendered when
 *    the default expanded state is updated on the parent, and
 *  * memo-wrapped child components will also be rerendered if they consume
 *    the default expanded state (via props or context) even if their local
 *    expanded state stay the same, and
 *  * an additional `useEffect` hook is needed to reset the local expanded state
 *    whenever the default expanded state is updated (store all entry state in
 *    the parent will lead to all entries being rerendered whenever an entry
 *    is toggled).
 *
 * If we store it in a MobX object,
 *  * only components whose expansion state is changed by the update will be
 *    rerendered, and
 *  * we can make components conditionally depends on the default expansion
 *    state (e.g. summary cell for a build with no summary isn't expandable).
 */
export const ExpandableEntriesState = types
  .model('ExpandableEntriesState', {
    defaultExpanded: false,
    _entryExpanded: types.optional(
      types.map(
        types.model({
          id: types.identifier,
          expanded: types.boolean,
        }),
      ),
      {},
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

export type ExpandableEntriesStateInstance = Instance<
  typeof ExpandableEntriesState
>;
