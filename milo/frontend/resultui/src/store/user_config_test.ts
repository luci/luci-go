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

import { expect } from 'chai';
import { Duration } from 'luxon';
import { destroy, types } from 'mobx-state-tree';
import sinon, { SinonFakeTimers } from 'sinon';

import { FakeStorage } from '../libs/test_utils/fake_storage';
import { BuildStepsConfig, UserConfig, V1_CACHE_KEY } from './user_config';

describe('BuildStepsConfig', () => {
  it('should set pins recursively', async () => {
    const store = BuildStepsConfig.create({});
    after(() => destroy(store));

    // When pinned is true, set pins for all ancestors.
    store.setStepPin('parent|step|child', true);
    expect(store.stepIsPinned('parent')).to.be.true;
    expect(store.stepIsPinned('parent|step')).to.be.true;
    expect(store.stepIsPinned('parent|step|child')).to.be.true;
    expect(store.stepIsPinned('parent|step|child2')).to.be.false;
    expect(store.stepIsPinned('parent|step2')).to.be.false;
    expect(store.stepIsPinned('parent|step2|child1')).to.be.false;

    store.setStepPin('parent|step|child2', true);
    expect(store.stepIsPinned('parent')).to.be.true;
    expect(store.stepIsPinned('parent|step')).to.be.true;
    expect(store.stepIsPinned('parent|step|child')).to.be.true;
    expect(store.stepIsPinned('parent|step|child2')).to.be.true;
    expect(store.stepIsPinned('parent|step2')).to.be.false;
    expect(store.stepIsPinned('parent|step2|child1')).to.be.false;

    store.setStepPin('parent|step2|child', true);
    expect(store.stepIsPinned('parent')).to.be.true;
    expect(store.stepIsPinned('parent|step')).to.be.true;
    expect(store.stepIsPinned('parent|step|child')).to.be.true;
    expect(store.stepIsPinned('parent|step|child2')).to.be.true;
    expect(store.stepIsPinned('parent|step2')).to.be.true;
    expect(store.stepIsPinned('parent|step2|child')).to.be.true;

    // When pinned is false, remove pins for all descendants.
    store.setStepPin('parent|step', false);
    expect(store.stepIsPinned('parent')).to.be.true;
    expect(store.stepIsPinned('parent|step')).to.be.false;
    expect(store.stepIsPinned('parent|step|child')).to.be.false;
    expect(store.stepIsPinned('parent|step|child2')).to.be.false;
    expect(store.stepIsPinned('parent|step2')).to.be.true;
    expect(store.stepIsPinned('parent|step2|child')).to.be.true;
  });
});

describe('UserConfig', () => {
  let timer: SinonFakeTimers;
  beforeEach(() => {
    timer = sinon.useFakeTimers();
  });
  afterEach(() => {
    timer.restore();
  });

  it('should recover from config v1 cache', () => {
    const storage = new FakeStorage();
    storage.setItem(
      V1_CACHE_KEY,
      JSON.stringify({
        steps: {
          showSucceededSteps: true,
          showDebugLogs: false,
          expandByDefault: true,
          stepPinTime: {
            parent: Date.now(),
            'parent|child': Date.now() - Duration.fromObject({ hour: 2 }).toMillis(),
          },
        },
        testResults: {
          columnWidths: {
            'v.column1': 300,
            'v.column2': 400,
          },
        },
        inputPropLineFoldTime: {
          inputKey1: Date.now(),
          inputKey2: Date.now() - Duration.fromObject({ hour: 2 }).toMillis(),
        },
        outputPropLineFoldTime: {
          outputKey1: Date.now(),
          outputKey2: Date.now() - Duration.fromObject({ hour: 2 }).toMillis(),
        },
        defaultBuildPageTabName: 'build-test-results',
      })
    );
    const transientKeysTTL = Duration.fromObject({ hour: 1 }).toMillis();
    const store = UserConfig.create({}, { storage, transientKeysTTL });
    after(() => destroy(store));

    expect(store.build.defaultTabName).to.eq('build-test-results');
    expect(store.build.inputProperties.isFolded('inputKey1')).to.be.true;
    expect(store.build.inputProperties.isFolded('inputKey2')).to.be.false;
    expect(store.build.outputProperties.isFolded('outputKey1')).to.be.true;
    expect(store.build.outputProperties.isFolded('outputKey2')).to.be.false;
    expect(store.build.steps.expandSucceededByDefault).to.be.true;
    expect(store.build.steps.showDebugLogs).to.be.false;
    expect(store.build.steps.showSucceededSteps).to.be.true;
    expect(store.build.steps.stepIsPinned('parent')).to.be.true;
    expect(store.build.steps.stepIsPinned('parent|child')).to.be.false;
    expect(store.tests.columnWidths).to.deep.eq({
      'v.test_suite': 350, // coming from the default V1 configuration.
      'v.column1': 300,
      'v.column2': 400,
    });

    expect(storage.getItem(V1_CACHE_KEY)).to.be.null;
  });

  it('should persist config to storage correctly', () => {
    const storage = new FakeStorage();
    const setItemSpy = sinon.spy(storage, 'setItem');
    const transientKeysTTL = Duration.fromObject({ hour: 2 }).toMillis();
    const store1 = UserConfig.create({}, { storage, transientKeysTTL });
    after(() => destroy(store1));

    store1.build.steps.setStepPin('parent|child', true);
    store1.build.steps.setStepPin('parent|child2', true);
    store1.build.inputProperties.setFolded('inputKey1', true);
    store1.build.inputProperties.setFolded('inputKey2', true);
    store1.build.outputProperties.setFolded('outputKey1', true);
    store1.build.outputProperties.setFolded('outputKey2', true);

    timer.runAll();
    timer.tick(Duration.fromObject({ hour: 1 }).toMillis());
    expect(setItemSpy.callCount).to.eq(1); // Writes should be batched together.

    store1.build.steps.setStepPin('parent|child2', true);
    store1.build.inputProperties.setFolded('inputKey2', true);
    store1.build.inputProperties.setFolded('inputKey3', true);
    store1.build.outputProperties.setFolded('outputKey2', true);
    store1.build.outputProperties.setFolded('outputKey3', true);

    timer.runAll();
    timer.tick(Duration.fromObject({ hour: 1 }).toMillis());
    expect(setItemSpy.callCount).to.eq(2); // Writes should be batched together.

    const store2 = UserConfig.create({}, { storage, transientKeysTTL });
    after(() => destroy(store2));

    // Keys that were stale.
    expect(store2.build.steps.stepIsPinned('parent|child')).to.be.false;
    expect(store2.build.inputProperties.isFolded('inputKey1')).to.be.false;
    expect(store2.build.outputProperties.isFolded('outputKey1')).to.be.false;

    // Keys that were recently created/updated.
    expect(store2.build.steps.stepIsPinned('parent|child2')).to.be.true;
    expect(store2.build.inputProperties.isFolded('inputKey2')).to.be.true;
    expect(store2.build.inputProperties.isFolded('inputKey3')).to.be.true;
    expect(store2.build.outputProperties.isFolded('outputKey2')).to.be.true;
    expect(store2.build.outputProperties.isFolded('outputKey3')).to.be.true;
  });

  it('should persist config to storage correctly when attached to another store', () => {
    const RootStore = types
      .model({
        userConfig: types.optional(UserConfig, {}),
      })
      .actions((self) => ({
        modifyDefaultBuildTabNameFromParent(tabName: string) {
          self.userConfig.build.setDefaultTab(tabName);
        },
      }));

    const storage = new FakeStorage();
    const store1 = RootStore.create({}, { storage });
    after(() => destroy(store1));

    store1.modifyDefaultBuildTabNameFromParent('new tab name');
    expect(store1.userConfig.build.defaultTabName).to.eq('new tab name');

    timer.runAll();
    timer.tick(Duration.fromObject({ hour: 1 }).toMillis());

    const store2 = UserConfig.create({}, { storage });
    after(() => destroy(store2));
    expect(store2.build.defaultTabName).to.eq('new tab name');
  });
});
