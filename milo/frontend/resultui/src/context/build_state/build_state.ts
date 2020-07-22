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
import { fromPromise, FULFILLED, IPromiseBasedObservable } from 'mobx-utils';

import { consumeContext, provideContext } from '../../libs/context';
import { BuilderID } from '../../services/buildbucket';
import { BuildPageData } from '../../services/build_page';
import { AppState } from '../app_state/app_state';

/**
 * Records state of a build.
 */
export class BuildState {
  @observable.ref builder?: BuilderID;
  @observable.ref buildNumOrId?: string;

  constructor(private appState: AppState) {}

  @computed
  get buildPageDataReq(): IPromiseBasedObservable<BuildPageData> {
    if (!this.appState.buildPageService || !this.builder || this.buildNumOrId === undefined) {
      // Returns a promise that never resolves when the dependencies aren't
      // ready.
      return fromPromise(new Promise(() => {}));
    }
    return fromPromise(this.appState.buildPageService.getBuildPageData({
      builder: this.builder,
      buildNumOrId: this.buildNumOrId,
    }));
  }

  @computed
  get buildPageData(): BuildPageData | null {
    if (this.buildPageDataReq.state !== FULFILLED) {
      return null;
    }
    return this.buildPageDataReq.value;
  }
}

export const consumeBuildState = consumeContext<'buildState', BuildState>('buildState');
export const provideBuildState = provideContext<'buildState', BuildState>('buildState');
