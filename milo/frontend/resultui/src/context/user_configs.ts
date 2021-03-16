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
import { observable } from 'mobx';

import { consumeContext, provideContext } from '../libs/context';

// Backward incompatible changes are not allowed.
export interface UserConfigs {
  hints: {
    showTestResultsHint: boolean;
  };
  steps: {
    showSucceededSteps: boolean;
    showDebugLogs: boolean;
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
  askForFeedback: boolean;
  defaultBuildPageTabName: string;
}

export const DEFAULT_USER_CONFIGS = Object.freeze<UserConfigs>({
  hints: {
    showTestResultsHint: true,
  },
  steps: Object.freeze({
    showSucceededSteps: true,
    showDebugLogs: false,
  }),
  inputPropLineFoldTime: Object.freeze({}),
  outputPropLineFoldTime: Object.freeze({}),
  askForFeedback: true,
  defaultBuildPageTabName: 'build-overview',
});

export class UserConfigsStore {
  private static readonly KEY = 'user-configs-v1';

  @observable readonly userConfigs = merge<{}, UserConfigs>({}, DEFAULT_USER_CONFIGS);

  constructor(private readonly storage = window.localStorage) {
    const storedConfigsStr = storage.getItem(UserConfigsStore.KEY) || '{}';
    merge(this.userConfigs, JSON.parse(storedConfigsStr));

    const fourWeeksAgo = Date.now() - 2419200000;
    this.deleteStaleKeys(this.userConfigs.inputPropLineFoldTime, fourWeeksAgo);
    this.deleteStaleKeys(this.userConfigs.outputPropLineFoldTime, fourWeeksAgo);
  }

  private deleteStaleKeys(records: { [key: string]: number }, beforeTimestamp: number) {
    Object.entries(records)
      .filter(([, timestamp]) => timestamp < beforeTimestamp)
      .forEach(([key]) => delete records[key]);
  }

  save() {
    this.storage.setItem(UserConfigsStore.KEY, JSON.stringify(this.userConfigs));
  }
}

export const consumeConfigsStore = consumeContext<'configsStore', UserConfigsStore>('configsStore');
export const provideConfigsStore = provideContext<'configsStore', UserConfigsStore>('configsStore');
