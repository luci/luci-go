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

import { Duration } from 'luxon';
import { destroy, Instance, isAlive, types } from 'mobx-state-tree';

import { FakeStorage } from '@/testing_tools/fakes/fake_storage';

import { UserConfig, UserConfigInstance } from './user_config';

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

  test('should persist config to storage correctly', () => {
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

  test('should persist config to storage correctly when attached to another store', () => {
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
