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
import { BuildExt } from '../../models/build_ext';
import { Build, BuilderID, GetBuildRequest } from '../../services/buildbucket';
import { RelatedBuildsData } from '../../services/build_page';
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
  get buildReq(): IPromiseBasedObservable<Build> {
    if (!this.appState.buildsService || !this.builder || this.buildNumOrId === undefined) {
      // Returns a promise that never resolves when the dependencies aren't
      // ready.
      return fromPromise(new Promise(() => {}));
    }
    // Since response can be different when queried at different time,
    // establish a dependency on timestamp.
    this.timestamp;  // tslint:disable-line: no-unused-expression

    const req: GetBuildRequest = this.buildNumOrId.startsWith('b')
      ? {id: this.buildNumOrId.slice(1), fields: '*'}
      : {builder: this.builder, buildNumber: Number(this.buildNumOrId), fields: '*'};

    return fromPromise(this.appState.buildsService.getBuild(req));
  }

  @computed
  get build(): BuildExt | null {
    if (this.buildReq.state !== FULFILLED) {
      return null;
    }
    return new BuildExt(this.buildReq.value);
  }

  @computed
  get isCanary(): boolean {
    return Boolean(this.build?.input.experiments?.includes('luci.non_production'));
  }

  @computed({keepAlive: true})
  get relatedBuildsDataReq(): IPromiseBasedObservable<RelatedBuildsData> {
    // Since response can be different when queried at different time,
    // establish a dependency on timestamp.
    this.timestamp;  // tslint:disable-line: no-unused-expression
    if (!this.appState.buildPageService || !this.build) {
      return fromPromise(new Promise(() => {}));
    }
    return fromPromise(this.appState.buildPageService.getRelatedBuilds(this.build.id));
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
    if (!this.appState.milo || !this.build) {
      return async function*() { await Promise.race([]); };
    }
    if (!this.build.input.gitilesCommit) {
      return async function*() {};
    }

    let req: QueryBlamelistRequest = {
      gitilesCommit: this.build.input.gitilesCommit,
      builder: this.build.builder,
    };
    const milo = this.appState.milo;
    async function* streamBlamelist() {
      let res: QueryBlamelistResponse;
      do {
        res = await milo.queryBlamelist(req);
        req = {...req, pageToken: res.nextPageToken};
        yield res;
      } while (res.nextPageToken);
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
