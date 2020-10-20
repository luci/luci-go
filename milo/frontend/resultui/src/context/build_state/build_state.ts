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
import * as iter from '../../libs/iter_utils';
import { BuilderID } from '../../services/buildbucket';
import { BuildPageData, RelatedBuildsData } from '../../services/build_page';
import { QueryBlamelistRequest, QueryBlamelistResponse } from '../../services/milo_internal';
import { AppState } from '../app_state/app_state';

/**
 * Records state of a build.
 */
export class BuildState {
  @observable.ref builder?: BuilderID;
  @observable.ref buildNumOrId?: string;

  @observable.ref private timestamp = Date.now();

  constructor(private appState: AppState) {}

  @computed
  get buildPageDataReq(): IPromiseBasedObservable<BuildPageData> {
    if (!this.appState.buildPageService || !this.builder || this.buildNumOrId === undefined) {
      // Returns a promise that never resolves when the dependencies aren't
      // ready.
      return fromPromise(new Promise(() => {}));
    }
    // Since response can be different when queried at different time,
    // establish a dependency on timestamp.
    this.timestamp;  // tslint:disable-line: no-unused-expression
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

  @computed
  get isCanary(): boolean {
    return Boolean(this.buildPageData?.input.experiments?.includes('luci.non_production'));
  }

  @computed({keepAlive: true})
  get relatedBuildsDataReq(): IPromiseBasedObservable<RelatedBuildsData> {
    // Since response can be different when queried at different time,
    // establish a dependency on timestamp.
    this.timestamp;  // tslint:disable-line: no-unused-expression
    if (!this.appState.buildPageService || !this.buildPageData) {
      return fromPromise(new Promise(() => {}));
    }
    return fromPromise(this.appState.buildPageService.getRelatedBuilds(this.buildPageData!.id));
  }

  @computed
  get relatedBuildsData(): RelatedBuildsData | null {
    if (this.relatedBuildsDataReq.state !== FULFILLED) {
      return null;
    }
    return this.relatedBuildsDataReq.value;
  }

  @computed({keepAlive: true})
  get queryBlamelistResIterFn() {
    if (!this.appState.milo || !this.buildPageData) {
      return async function*() { await Promise.race([]); };
    }
    if (!this.buildPageData.input.gitiles_commit) {
      return async function*() {};
    }

    let req: QueryBlamelistRequest = {
      gitiles_commit: this.buildPageData.input.gitiles_commit,
      builder: this.buildPageData.builder,
    };
    const milo = this.appState.milo;
    async function* streamBlamelist() {
      let res: QueryBlamelistResponse;
      do {
        res = await milo.queryBlamelist(req);
        req = {...req, page_token: res.next_page_token};
        yield res;
      } while (res.next_page_token);
    }
    return iter.teeAsync(streamBlamelist());
  }

  // Refresh all data that depends on the timestamp.
  refresh() {
    this.timestamp = Date.now();
  }
}

export const consumeBuildState = consumeContext<'buildState', BuildState>('buildState');
export const provideBuildState = provideContext<'buildState', BuildState>('buildState');
