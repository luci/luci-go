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

import { expect } from 'chai';
import { destroy } from 'mobx-state-tree';

import { ExpandableEntriesState } from './expandable_entries_state';

describe('ExpandableEntriesState', () => {
  it('e2e', () => {
    const state = ExpandableEntriesState.create();
    after(() => destroy(state));

    // Toggle all to false.
    state.toggleAll(false);
    expect(state.defaultExpanded).to.be.false;
    expect(state.isExpanded('entry1')).to.be.false;
    expect(state.isExpanded('entry2')).to.be.false;
    expect(state.isExpanded('entry3')).to.be.false;

    // Toggle some entries.
    state.toggle('entry1', true);
    state.toggle('entry2', true);
    expect(state.defaultExpanded).to.be.false;
    expect(state.isExpanded('entry1')).to.be.true;
    expect(state.isExpanded('entry2')).to.be.true;
    expect(state.isExpanded('entry3')).to.be.false;

    // Toggle all entries when some entries are toggled.
    state.toggleAll(false);
    expect(state.defaultExpanded).to.be.false;
    expect(state.isExpanded('entry1')).to.be.false;
    expect(state.isExpanded('entry2')).to.be.false;
    expect(state.isExpanded('entry3')).to.be.false;

    // Toggle all to true.
    state.toggleAll(true);
    expect(state.defaultExpanded).to.be.true;
    expect(state.isExpanded('entry1')).to.be.true;
    expect(state.isExpanded('entry2')).to.be.true;
    expect(state.isExpanded('entry3')).to.be.true;

    // Toggle some entries.
    state.toggle('entry1', false);
    state.toggle('entry3', false);
    expect(state.defaultExpanded).to.be.true;
    expect(state.isExpanded('entry1')).to.be.false;
    expect(state.isExpanded('entry2')).to.be.true;
    expect(state.isExpanded('entry3')).to.be.false;
  });
});
