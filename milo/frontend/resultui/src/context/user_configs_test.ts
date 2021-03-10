// Copyright 2021 The LUCI Authors.
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

import { assert } from 'chai';

import { UserConfigs, UserConfigsStore } from './user_configs';


describe('UserConfigs', () => {
  it('should delete stale propLineFoldTime records', () => {
    let savedConfigs!: UserConfigs;
    const fourWeeksAgo = Date.now() - 2419200000;
    const store = new UserConfigsStore({
      getItem: () => JSON.stringify({
        inputPropLineFoldTime: {
          'old-line': fourWeeksAgo - 360000,
          'new-line': fourWeeksAgo + 360000,
        },
        outputPropLineFoldTime: {
          'old-line': fourWeeksAgo - 360000,
          'new-line': fourWeeksAgo + 360000,
        },
      }),
      setItem: (_key, value) => savedConfigs = JSON.parse(value),
    } as Partial<Storage> as Storage);
    store.save();

    assert.notInclude(Object.keys(store.userConfigs.inputPropLineFoldTime), 'old-line');
    assert.notInclude(Object.keys(store.userConfigs.outputPropLineFoldTime), 'old-line');
    assert.include(Object.keys(store.userConfigs.inputPropLineFoldTime), 'new-line');
    assert.include(Object.keys(store.userConfigs.outputPropLineFoldTime), 'new-line');

    assert.notInclude(Object.keys(savedConfigs.inputPropLineFoldTime), 'old-line');
    assert.notInclude(Object.keys(savedConfigs.outputPropLineFoldTime), 'old-line');
    assert.include(Object.keys(savedConfigs.inputPropLineFoldTime), 'new-line');
    assert.include(Object.keys(savedConfigs.outputPropLineFoldTime), 'new-line');
  });
});
