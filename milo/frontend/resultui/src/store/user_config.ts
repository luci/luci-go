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

import { debounce, merge } from 'lodash-es';
import { Duration } from 'luxon';
import {
  addDisposer,
  addMiddleware,
  applySnapshot,
  getEnv,
  getSnapshot,
  Instance,
  SnapshotIn,
  SnapshotOut,
  types,
} from 'mobx-state-tree';

import { TransientKeySet } from './transient_key_set';

export const V1_CACHE_KEY = 'user-configs-v1';
export const V2_CACHE_KEY = 'user-config-v2';
const DEFAULT_TRANSIENT_KEY_TTL = Duration.fromObject({ week: 4 }).toMillis();

export interface UserConfigEnv {
  readonly storage?: Storage;
  readonly transientKeysTTL?: number;
}

interface UserConfigsV1 {
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

export const DEFAULT_USER_CONFIGS_V1 = Object.freeze<UserConfigsV1>({
  steps: Object.freeze({
    showSucceededSteps: false,
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

function configV1ToV2(v1: UserConfigsV1): UserConfigSnapshotIn {
  return {
    build: {
      steps: {
        showSucceededSteps: v1.steps.showSucceededSteps,
        showDebugLogs: v1.steps.showDebugLogs,
        expandSucceededByDefault: v1.steps.expandByDefault,
        _pinnedSteps: {
          _values: v1.steps.stepPinTime,
        },
      },
      inputProperties: {
        _foldedRootKeys: {
          _values: v1.inputPropLineFoldTime,
        },
      },
      outputProperties: {
        _foldedRootKeys: {
          _values: v1.outputPropLineFoldTime,
        },
      },
      defaultTabName: v1.defaultBuildPageTabName,
    },
    tests: {
      _columnWidths: v1.testResults?.columnWidths,
    },
  };
}

export const BuildStepsConfig = types
  .model('BuildStepsConfig', {
    showSucceededSteps: false,
    showDebugLogs: false,
    expandSucceededByDefault: false,
    /**
     * SHOULD NOT BE ACCESSED DIRECTLY. Use the methods instead.
     */
    // Map<StepName, LastUpdated>
    _pinnedSteps: types.optional(TransientKeySet, {}),
  })
  .views((self) => ({
    stepIsPinned(step: string) {
      return self._pinnedSteps.has(step);
    },
  }))
  .actions((self) => ({
    setShowSucceededSteps(show: boolean) {
      self.showSucceededSteps = show;
    },
    setShowDebugLogs(show: boolean) {
      self.showDebugLogs = show;
    },
    setExpandSucceededByDefault(expand: boolean) {
      self.expandSucceededByDefault = expand;
    },

    /**
     * Pin/unpin a step.
     * The configuration is shared across all builds.
     *
     * When pinning a step, pin all its ancestors as well.
     * When unpinning a step, unpin all its descendants as well.
     */
    setStepPin(targetStepName: string, pinned: boolean) {
      if (pinned) {
        let stepName = '';
        for (const subStepName of targetStepName.split('|')) {
          if (stepName) {
            stepName += '|';
          }
          stepName += subStepName;
          self._pinnedSteps.add(stepName);
        }
      } else {
        self._pinnedSteps.delete(targetStepName);
        const prefix = targetStepName + '|';
        for (const stepName of self._pinnedSteps.keys()) {
          if (stepName.startsWith(prefix)) {
            self._pinnedSteps.delete(stepName);
          }
        }
      }
    },
    deleteStaleKeys(before: Date) {
      self._pinnedSteps.deleteStaleKeys(before);
    },
  }));

export const PropertyViewerConfig = types
  .model('PropertyViewerConfig', {
    /**
     * SHOULD NOT BE ACCESSED DIRECTLY. Use the methods instead.
     */
    // Map<FoldedRootKey, LastUpdated>
    _foldedRootKeys: types.optional(TransientKeySet, {}),
  })
  .views((self) => ({
    isFolded(key: string) {
      return self._foldedRootKeys.has(key);
    },
  }))
  .actions((self) => ({
    setFolded(key: string, folded: boolean) {
      if (folded) {
        self._foldedRootKeys.add(key);
      } else {
        self._foldedRootKeys.delete(key);
      }
    },
    deleteStaleKeys(before: Date) {
      self._foldedRootKeys.deleteStaleKeys(before);
    },
  }));

export type PropertyViewerConfigInstance = Instance<typeof PropertyViewerConfig>;
export type PropertyViewerConfigSnapshotIn = SnapshotIn<typeof PropertyViewerConfig>;
export type PropertyViewerConfigSnapshotOut = SnapshotOut<typeof PropertyViewerConfig>;

export const BuildConfig = types
  .model('BuildConfig', {
    steps: types.optional(BuildStepsConfig, {}),
    inputProperties: types.optional(PropertyViewerConfig, {}),
    outputProperties: types.optional(PropertyViewerConfig, {}),
    defaultTabName: 'build-overview',
  })
  .actions((self) => ({
    setDefaultTab(tabName: string) {
      self.defaultTabName = tabName;
    },
    deleteStaleKeys(before: Date) {
      self.steps.deleteStaleKeys(before);
      self.inputProperties.deleteStaleKeys(before);
      self.outputProperties.deleteStaleKeys(before);
    },
  }));

export const TestsConfig = types
  .model('TestsConfig', {
    /**
     * SHOULD NOT BE ACCESSED DIRECTLY. Use the methods instead.
     */
    // Map<ColumnKey, ColumnWidthInPx>
    _columnWidths: types.optional(types.map(types.number), { 'v.test_suite': 350 }),
  })
  .views((self) => ({
    get columnWidths() {
      return Object.fromEntries(self._columnWidths.entries());
    },
  }))
  .actions((self) => ({
    setColumWidth(key: string, width: number) {
      self._columnWidths.set(key, width);
    },
  }));

export const UserConfig = types
  .model('UserConfig', {
    id: types.optional(types.identifier, () => `UserConfig/${Math.random()}`),
    build: types.optional(BuildConfig, {}),
    tests: types.optional(TestsConfig, {}),
  })
  .actions((self) => ({
    deleteStaleKeys(before: Date) {
      self.build.deleteStaleKeys(before);
    },
    restoreFromConfigV1(storage: Storage) {
      try {
        const configV1Str = storage.getItem(V1_CACHE_KEY);
        if (configV1Str === null) {
          return;
        }
        const configV1 = merge({}, DEFAULT_USER_CONFIGS_V1, JSON.parse(configV1Str)) as UserConfigsV1;
        const configV2 = configV1ToV2(configV1);
        applySnapshot(self, { ...configV2, id: self.id });
      } catch (e) {
        console.error(e);
        console.warn('encountered an error when restoring user configs from the v1, ignoring it');
      }
      storage.removeItem(V1_CACHE_KEY);
      storage.setItem(V2_CACHE_KEY, JSON.stringify(getSnapshot(self)));
    },
    restoreConfig(storage: Storage) {
      try {
        const snapshotStr = storage.getItem(V2_CACHE_KEY);
        if (snapshotStr === null) {
          return;
        }
        applySnapshot(self, { ...JSON.parse(snapshotStr), id: self.id });
      } catch (e) {
        console.error(e);
        console.warn('encountered an error when restoring user configs from the cache, deleting it');
        storage.removeItem(V2_CACHE_KEY);
      }
    },
    enableCaching() {
      const env: UserConfigEnv = getEnv(self) || {};
      const storage = env.storage || window.localStorage;
      const ttl = env.transientKeysTTL || DEFAULT_TRANSIENT_KEY_TTL;

      this.restoreFromConfigV1(storage);
      this.restoreConfig(storage);
      this.deleteStaleKeys(new Date(Date.now() - ttl));

      const persistConfig = debounce(
        () => storage.setItem(V2_CACHE_KEY, JSON.stringify(getSnapshot(self))),
        // Add a tiny delay so updates happened in the same event cycle will
        // only trigger one save event.
        1
      );

      addDisposer(
        self,
        // We cannot use `onAction` because it will not intercept actions
        // initiated on the ancestor nodes, even if those actions calls the
        // actions on this node.
        // Use `addMiddleware` allows use to intercept any action acted on this
        // node (and its descendent). However, if the parent decided to modify
        // the child node without calling its action, we still can't intercept
        // it. Currently, there's no way around this.
        //
        // See https://github.com/mobxjs/mobx-state-tree/issues/1948
        addMiddleware(self, (call, next) => {
          next(call, (value) => {
            // persistConfig AFTER the action is applied.
            persistConfig();
            return value;
          });
        })
      );
    },
  }));

export type UserConfigInstance = Instance<typeof UserConfig>;
export type UserConfigSnapshotIn = SnapshotIn<typeof UserConfig>;
export type UserConfigSnapshotOut = SnapshotOut<typeof UserConfig>;
