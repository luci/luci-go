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

import { afterEach, beforeEach, expect } from '@jest/globals';
import { destroy } from 'mobx-state-tree';

import { ExpandableEntriesState, ExpandableEntriesStateInstance } from './expandable_entries_state';

describe('ExpandableEntriesState', () => {
  let state: ExpandableEntriesStateInstance;

  beforeEach(() => {
    state = ExpandableEntriesState.create();
  });
  afterEach(() => {
    destroy(state);
  });

  it('e2e', () => {
    // Toggle all to false.
    state.toggleAll(false);
    expect(state.defaultExpanded).toBeFalsy();
    expect(state.isExpanded('entry1')).toBeFalsy();
    expect(state.isExpanded('entry2')).toBeFalsy();
    expect(state.isExpanded('entry3')).toBeFalsy();

    // Toggle some entries.
    state.toggle('entry1', true);
    state.toggle('entry2', true);
    expect(state.defaultExpanded).toBeFalsy();
    expect(state.isExpanded('entry1')).toBeTruthy();
    expect(state.isExpanded('entry2')).toBeTruthy();
    expect(state.isExpanded('entry3')).toBeFalsy();

    // Toggle all entries when some entries are toggled.
    state.toggleAll(false);
    expect(state.defaultExpanded).toBeFalsy();
    expect(state.isExpanded('entry1')).toBeFalsy();
    expect(state.isExpanded('entry2')).toBeFalsy();
    expect(state.isExpanded('entry3')).toBeFalsy();

    // Toggle all to true.
    state.toggleAll(true);
    expect(state.defaultExpanded).toBeTruthy();
    expect(state.isExpanded('entry1')).toBeTruthy();
    expect(state.isExpanded('entry2')).toBeTruthy();
    expect(state.isExpanded('entry3')).toBeTruthy();

    // Toggle some entries.
    state.toggle('entry1', false);
    state.toggle('entry3', false);
    expect(state.defaultExpanded).toBeTruthy();
    expect(state.isExpanded('entry1')).toBeFalsy();
    expect(state.isExpanded('entry2')).toBeTruthy();
    expect(state.isExpanded('entry3')).toBeFalsy();
  });
});
