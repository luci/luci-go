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

import { afterEach, beforeEach, expect, jest } from '@jest/globals';
import { Duration } from 'luxon';
import { destroy, Instance, isAlive, types } from 'mobx-state-tree';

import { FakeStorage } from '@/testing_tools/fakes/fake_storage';

import {
  BuildStepsConfig,
  BuildStepsConfigInstance,
  UserConfig,
  UserConfigInstance,
  V1_CACHE_KEY,
  V2_CACHE_KEY,
} from './user_config';

describe('BuildStepsConfig', () => {
  let store: BuildStepsConfigInstance;

  beforeEach(() => {
    store = BuildStepsConfig.create({});
  });
  afterEach(() => {
    destroy(store);
  });

  it('should set pins recursively', async () => {
    // When pinned is true, set pins for all ancestors.
    store.setStepPin('parent|step|child', true);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2|child1')).toBeFalsy();

    store.setStepPin('parent|step|child2', true);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2|child1')).toBeFalsy();

    store.setStepPin('parent|step2|child', true);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child')).toBeTruthy();
    expect(store.stepIsPinned('parent|step|child2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2|child')).toBeTruthy();

    // When pinned is false, remove pins for all descendants.
    store.setStepPin('parent|step', false);
    expect(store.stepIsPinned('parent')).toBeTruthy();
    expect(store.stepIsPinned('parent|step')).toBeFalsy();
    expect(store.stepIsPinned('parent|step|child')).toBeFalsy();
    expect(store.stepIsPinned('parent|step|child2')).toBeFalsy();
    expect(store.stepIsPinned('parent|step2')).toBeTruthy();
    expect(store.stepIsPinned('parent|step2|child')).toBeTruthy();
  });
});

describe('UserConfig', () => {
  const RootStore = types
    .model({
      userConfig: types.optional(UserConfig, {}),
    })
    .actions((self) => ({
      modifyDefaultBuildTabNameFromParent(tab: string) {
        self.userConfig.build.setDefaultTab(tab);
      },
      afterCreate() {
        self.userConfig.enableCaching();
      },
    }));

  let store1: UserConfigInstance | null = null;
  let store2: UserConfigInstance | null = null;
  let rootStore: Instance<typeof RootStore> | null = null;

  beforeEach(() => {
    jest.useFakeTimers();
  });
  afterEach(() => {
    jest.useRealTimers();
    if (store1 && isAlive(store1)) {
      destroy(store1);
    }
    if (store2 && isAlive(store2)) {
      destroy(store2);
    }
    if (rootStore && isAlive(rootStore)) {
      destroy(rootStore);
    }
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
            'parent|child':
              Date.now() - Duration.fromObject({ hour: 2 }).toMillis(),
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
    store1 = UserConfig.create({}, { storage, transientKeysTTL });
    store1.enableCaching();

    expect(store1.build.defaultTab).toStrictEqual('build-test-results');
    expect(store1.build.inputProperties.isFolded('inputKey1')).toBeTruthy();
    expect(store1.build.inputProperties.isFolded('inputKey2')).toBeFalsy();
    expect(store1.build.outputProperties.isFolded('outputKey1')).toBeTruthy();
    expect(store1.build.outputProperties.isFolded('outputKey2')).toBeFalsy();
    expect(store1.build.steps.showDebugLogs).toBeFalsy();
    expect(store1.build.steps.stepIsPinned('parent')).toBeTruthy();
    expect(store1.build.steps.stepIsPinned('parent|child')).toBeFalsy();
    expect(store1.tests.columnWidths).toEqual({
      'v.test_suite': 350, // coming from the default V1 configuration.
      'v.column1': 300,
      'v.column2': 400,
    });
    expect(storage.getItem(V1_CACHE_KEY)).toBeNull();

    // Should store the config to the new cache location even when there's no
    // user initiated action.
    expect(storage.getItem(V2_CACHE_KEY)).not.toBeNull();
    store2 = UserConfig.create({}, { storage, transientKeysTTL });
    store2.enableCaching();
    expect(store2.build.defaultTab).toStrictEqual('build-test-results');
    expect(store2.build.inputProperties.isFolded('inputKey1')).toBeTruthy();
    expect(store2.build.inputProperties.isFolded('inputKey2')).toBeFalsy();
    expect(store2.build.outputProperties.isFolded('outputKey1')).toBeTruthy();
    expect(store2.build.outputProperties.isFolded('outputKey2')).toBeFalsy();
    expect(store2.build.steps.showDebugLogs).toBeFalsy();
    expect(store2.build.steps.stepIsPinned('parent')).toBeTruthy();
    expect(store2.build.steps.stepIsPinned('parent|child')).toBeFalsy();
    expect(store2.tests.columnWidths).toEqual({
      'v.test_suite': 350, // coming from the default V1 configuration.
      'v.column1': 300,
      'v.column2': 400,
    });
  });

  it('should persist config to storage correctly', () => {
    const storage = new FakeStorage();
    const setItemSpy = jest.spyOn(storage, 'setItem');
    const transientKeysTTL = Duration.fromObject({ hour: 2 }).toMillis();
    store1 = UserConfig.create({}, { storage, transientKeysTTL });
    store1.enableCaching();

    store1.build.steps.setStepPin('parent|child', true);
    store1.build.steps.setStepPin('parent|child2', true);
    store1.build.inputProperties.setFolded('inputKey1', true);
    store1.build.inputProperties.setFolded('inputKey2', true);
    store1.build.outputProperties.setFolded('outputKey1', true);
    store1.build.outputProperties.setFolded('outputKey2', true);

    jest.runAllTimers();
    jest.advanceTimersByTime(Duration.fromObject({ hour: 1 }).toMillis());
    expect(setItemSpy.mock.calls.length).toStrictEqual(1); // Writes should be batched together.

    store1.build.steps.setStepPin('parent|child2', true);
    store1.build.inputProperties.setFolded('inputKey2', true);
    store1.build.inputProperties.setFolded('inputKey3', true);
    store1.build.outputProperties.setFolded('outputKey2', true);
    store1.build.outputProperties.setFolded('outputKey3', true);

    jest.runAllTimers();
    jest.advanceTimersByTime(Duration.fromObject({ hour: 1 }).toMillis());
    expect(setItemSpy.mock.calls.length).toStrictEqual(2); // Writes should be batched together.

    store2 = UserConfig.create({}, { storage, transientKeysTTL });
    store2.enableCaching();

    // Keys that were stale.
    expect(store2.build.steps.stepIsPinned('parent|child')).toBeFalsy();
    expect(store2.build.inputProperties.isFolded('inputKey1')).toBeFalsy();
    expect(store2.build.outputProperties.isFolded('outputKey1')).toBeFalsy();

    // Keys that were recently created/updated.
    expect(store2.build.steps.stepIsPinned('parent|child2')).toBeTruthy();
    expect(store2.build.inputProperties.isFolded('inputKey2')).toBeTruthy();
    expect(store2.build.inputProperties.isFolded('inputKey3')).toBeTruthy();
    expect(store2.build.outputProperties.isFolded('outputKey2')).toBeTruthy();
    expect(store2.build.outputProperties.isFolded('outputKey3')).toBeTruthy();
  });

  it('should persist config to storage correctly when attached to another store', () => {
    const storage = new FakeStorage();
    rootStore = RootStore.create({}, { storage });

    rootStore.modifyDefaultBuildTabNameFromParent('new tab name');
    expect(rootStore.userConfig.build.defaultTab).toStrictEqual('new tab name');

    jest.runAllTimers();
    jest.advanceTimersByTime(Duration.fromObject({ hour: 1 }).toMillis());

    store2 = UserConfig.create({}, { storage });
    store2.enableCaching();
    expect(store2.build.defaultTab).toStrictEqual('new tab name');
  });
});
