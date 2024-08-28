// Copyright 2023 The LUCI Authors.
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

import { Instance, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';

import { TransientKeySet } from '@/common/store/transient_key_set';

export const enum ExpandStepOption {
  All,
  WithNonSuccessful,
  NonSuccessful,
  None,
}

export const BuildStepsConfig = types
  .model('BuildStepsConfig', {
    elideSucceededSteps: true,
    expandByDefault: ExpandStepOption.WithNonSuccessful,
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
    setElideSucceededSteps(elide: boolean) {
      self.elideSucceededSteps = elide;
    },
    setExpandByDefault(opt: ExpandStepOption) {
      self.expandByDefault = opt;
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

export type BuildStepsConfigInstance = Instance<typeof BuildStepsConfig>;

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

export type PropertyViewerConfigInstance = Instance<
  typeof PropertyViewerConfig
>;
export type PropertyViewerConfigSnapshotIn = SnapshotIn<
  typeof PropertyViewerConfig
>;
export type PropertyViewerConfigSnapshotOut = SnapshotOut<
  typeof PropertyViewerConfig
>;

export const BuildConfig = types
  .model('BuildConfig', {
    steps: types.optional(BuildStepsConfig, {}),
    inputProperties: types.optional(PropertyViewerConfig, {}),
    outputProperties: types.optional(PropertyViewerConfig, {}),
    defaultTab: '',
  })
  .actions((self) => ({
    setDefaultTab(tab: string) {
      self.defaultTab = tab;
    },
    deleteStaleKeys(before: Date) {
      self.steps.deleteStaleKeys(before);
      self.inputProperties.deleteStaleKeys(before);
      self.outputProperties.deleteStaleKeys(before);
    },
  }));
