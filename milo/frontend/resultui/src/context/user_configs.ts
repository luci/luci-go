// Copyright 2020 The LUCI Authors.
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

import merge from 'lodash-es/merge';
import { computed, makeObservable, observable, reaction } from 'mobx';

import { createContextLink } from '../libs/context';

// Backward incompatible changes are not allowed.
export interface UserConfigs {
  steps: {
    showSucceededSteps: boolean;
    showDebugLogs: boolean;
    expandByDefault: boolean;
    stepPinTime: {
      // the key is the folded line.
      // the value is the last accessed time.
      [key: string]: number;
    };
  };
  testResults: {
    columnWidths: {
      // the key is the property key.
      // the value is the width in px.
      [key: string]: number;
    };
  };
  inputPropLineFoldTime: {
    // the key is the folded line.
    // the value is the last accessed time.
    [key: string]: number;
  };
  outputPropLineFoldTime: {
    // the key is the folded line.
    // the value is the last accessed time.
    [key: string]: number;
  };
  defaultBuildPageTabName: string;
}

export const DEFAULT_USER_CONFIGS = Object.freeze<UserConfigs>({
  steps: Object.freeze({
    showSucceededSteps: true,
    showDebugLogs: false,
    expandByDefault: false,
    stepPinTime: Object.freeze({}),
  }),
  testResults: {
    columnWidths: {
      'v.test_suite': 350,
    },
  },
  inputPropLineFoldTime: Object.freeze({}),
  outputPropLineFoldTime: Object.freeze({}),
  defaultBuildPageTabName: 'build-overview',
});

export class UserConfigsStore {
  private static readonly KEY = 'user-configs-v1';

  @observable readonly userConfigs = merge<{}, UserConfigs>({}, DEFAULT_USER_CONFIGS);

  private disposers: Array<() => void> = [];

  constructor(private readonly storage = window.localStorage) {
    makeObservable(this);

    const storedConfigsStr = storage.getItem(UserConfigsStore.KEY) || '{}';
    try {
      merge(this.userConfigs, JSON.parse(storedConfigsStr));
    } catch (e) {
      console.error(e);
      console.warn('encountered an error when parsing stored configs, deleting it');
      storage.removeItem(UserConfigsStore.KEY);
    }

    this.disposers.push(
      reaction(
        () => JSON.stringify(this.userConfigs),
        (configStr) => {
          this.storage.setItem(UserConfigsStore.KEY, configStr);
        },
        // Add a tiny delay so updates happened in the same event cycle will
        // only trigger one save event.
        { delay: 1 }
      )
    );

    const fourWeeksAgo = Date.now() - 2419200000;
    this.deleteStaleKeys(this.userConfigs.steps.stepPinTime, fourWeeksAgo);
    this.deleteStaleKeys(this.userConfigs.inputPropLineFoldTime, fourWeeksAgo);
    this.deleteStaleKeys(this.userConfigs.outputPropLineFoldTime, fourWeeksAgo);
  }

  /**
   * Perform cleanup.
   * Must be called before the object is GCed.
   */
  dispose() {
    this.disposers.reverse().forEach((disposer) => disposer());
    this.disposers = [];
  }

  private deleteStaleKeys(records: { [key: string]: number }, beforeTimestamp: number) {
    Object.entries(records)
      .filter(([, timestamp]) => timestamp < beforeTimestamp)
      .forEach(([key]) => delete records[key]);
  }

  /**
   * Whether a step is pinned.
   * The configuration is shared across all builds.
   */
  stepIsPinned(stepName: string) {
    const isPinned = computed(() => !!this.userConfigs.steps.stepPinTime[stepName]).get();
    return isPinned;
  }

  /**
   * Pin/unpin a step.
   * The configuration is shared across all builds.
   *
   * When pinning a step, pin all its ancestors as well.
   * When unpinning a step, unpin all its descendants as well.
   */
  setStepPin(targetStepName: string, pinned: boolean) {
    if (pinned) {
      const now = Date.now();
      let stepName = '';
      for (const subStepName of targetStepName.split('|')) {
        if (stepName) {
          stepName += '|';
        }
        stepName += subStepName;
        this.userConfigs.steps.stepPinTime[stepName] = now;
      }
    } else {
      delete this.userConfigs.steps.stepPinTime[targetStepName];
      const prefix = targetStepName + '|';
      // TODO(weiweilin): use a trie instead of a dictionary if this is too slow.
      Object.keys(this.userConfigs.steps.stepPinTime)
        .filter((stepName) => stepName.startsWith(prefix))
        .forEach((stepName) => {
          delete this.userConfigs.steps.stepPinTime[stepName];
        });
    }
  }
}

export const [provideConfigsStore, consumeConfigsStore] = createContextLink<UserConfigsStore>();
