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

import { aTimeout } from '@open-wc/testing/index-no-side-effects';
import { assert } from 'chai';
import sinon from 'sinon';

import { UserConfigs, UserConfigsStore } from './user_configs';

const FOUR_WEEKS_AGO = Date.now() - 2419200000;
const ONE_HOUR = 360000;

describe('UserConfigs', () => {
  it('should delete stale records', async () => {
    let savedConfigs!: UserConfigs;
    const store = new UserConfigsStore(({
      getItem: () =>
        JSON.stringify({
          steps: {
            stepPinTime: {
              'old-step': FOUR_WEEKS_AGO - ONE_HOUR,
              'new-step': FOUR_WEEKS_AGO + ONE_HOUR,
            },
          },
          inputPropLineFoldTime: {
            'old-line': FOUR_WEEKS_AGO - ONE_HOUR,
            'new-line': FOUR_WEEKS_AGO + ONE_HOUR,
          },
          outputPropLineFoldTime: {
            'old-line': FOUR_WEEKS_AGO - ONE_HOUR,
            'new-line': FOUR_WEEKS_AGO + ONE_HOUR,
          },
        }),
      setItem: (_key, value) => (savedConfigs = JSON.parse(value)),
    } as Partial<Storage>) as Storage);
    after(() => store.dispose());
    await aTimeout(20);

    assert.notInclude(Object.keys(store.userConfigs.steps.stepPinTime), 'old-step');
    assert.notInclude(Object.keys(store.userConfigs.inputPropLineFoldTime), 'old-line');
    assert.notInclude(Object.keys(store.userConfigs.outputPropLineFoldTime), 'old-line');
    assert.include(Object.keys(store.userConfigs.steps.stepPinTime), 'new-step');
    assert.include(Object.keys(store.userConfigs.inputPropLineFoldTime), 'new-line');
    assert.include(Object.keys(store.userConfigs.outputPropLineFoldTime), 'new-line');

    assert.notInclude(Object.keys(savedConfigs.steps.stepPinTime), 'old-step');
    assert.notInclude(Object.keys(savedConfigs.inputPropLineFoldTime), 'old-line');
    assert.notInclude(Object.keys(savedConfigs.outputPropLineFoldTime), 'old-line');
    assert.include(Object.keys(savedConfigs.steps.stepPinTime), 'new-step');
    assert.include(Object.keys(savedConfigs.inputPropLineFoldTime), 'new-line');
    assert.include(Object.keys(savedConfigs.outputPropLineFoldTime), 'new-line');
  });

  describe('step pin', () => {
    it('should return the correct step pin status', () => {
      const store = new UserConfigsStore(({
        getItem: () =>
          JSON.stringify({
            steps: {
              stepPinTime: {
                'new-step': FOUR_WEEKS_AGO + ONE_HOUR,
                'old-step': FOUR_WEEKS_AGO - ONE_HOUR,
              },
            },
          }),
        setItem: () => {},
      } as Partial<Storage>) as Storage);
      after(() => store.dispose());

      assert.isTrue(store.stepIsPinned('new-step'));
      assert.isFalse(store.stepIsPinned('old-step'));
    });

    it('should set pins recursively', async () => {
      const saveConfigSpy = sinon.spy();
      const store = new UserConfigsStore(({
        getItem: () => JSON.stringify({}),
        setItem: saveConfigSpy,
      } as Partial<Storage>) as Storage);
      after(() => store.dispose());

      // When pinned is true, set pins for all ancestors.
      store.setStepPin('parent|step|child', true);
      await aTimeout(20);
      assert.isTrue(store.stepIsPinned('parent'));
      assert.isTrue(store.stepIsPinned('parent|step'));
      assert.isTrue(store.stepIsPinned('parent|step|child'));
      assert.strictEqual(saveConfigSpy.callCount, 1);

      store.setStepPin('parent|step|child2', true);
      await aTimeout(20);
      assert.isTrue(store.stepIsPinned('parent|step|child2'));
      assert.strictEqual(saveConfigSpy.callCount, 2);

      store.setStepPin('parent|step2|child1', true);
      await aTimeout(20);
      assert.isTrue(store.stepIsPinned('parent|step2'));
      assert.isTrue(store.stepIsPinned('parent|step2|child1'));
      assert.strictEqual(saveConfigSpy.callCount, 3);

      // When pinned is false, remove pins for all descendants.
      store.setStepPin('parent|step', false);
      await aTimeout(20);
      assert.isTrue(store.stepIsPinned('parent'));
      assert.isFalse(store.stepIsPinned('parent|step'));
      assert.isFalse(store.stepIsPinned('parent|step|child'));
      assert.isFalse(store.stepIsPinned('parent|step|child2'));
      assert.isTrue(store.stepIsPinned('parent|step2'));
      assert.isTrue(store.stepIsPinned('parent|step2|child1'));
      assert.strictEqual(saveConfigSpy.callCount, 4);
    });
  });
});
