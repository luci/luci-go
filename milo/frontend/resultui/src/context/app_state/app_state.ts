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

import { computed, observable } from 'mobx';

import { consumeContext, provideContext } from '../../libs/context';
import { ResultDb } from '../../services/resultdb';

/**
 * Records the app-level state.
 */
export class AppState {
  @observable.ref accessToken = '';
  @observable.ref resultDbHost = '';
  @observable.ref selectedTabId = '';

  @computed
  get resultDb(): ResultDb | null {
    if (!this.accessToken || !this.resultDbHost) {
      return null;
    }
    return new ResultDb(this.resultDbHost, this.accessToken);
  }
}

export const consumeAppState = consumeContext<'appState', AppState>('appState');
export const provideAppState = provideContext<'appState', AppState>('appState');
