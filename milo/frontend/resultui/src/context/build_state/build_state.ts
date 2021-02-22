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
import { getGitilesRepoURL } from '../../libs/build_utils';

import { consumeContext, provideContext } from '../../libs/context';
import * as iter from '../../libs/iter_utils';
import { BuildExt } from '../../models/build_ext';
import { Build, BuilderID, GetBuildRequest, GitilesCommit } from '../../services/buildbucket';
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

  @observable.ref private isDisposed = false;

  /**
   * Perform cleanup.
   * Must be called before the object is GCed.
   */
  dispose() {
    this.isDisposed = true;

    // Evaluates @computed({keepAlive: true}) properties after this.isDisposed
    // is set to true so they no longer subscribes to any external observable.
    // tslint:disable: no-unused-expression
    this.relatedBuilds;
    this.queryBlamelistResIterFns;
    // tslint:enable: no-unused-expression
  }

  @computed
  get build$(): IPromiseBasedObservable<Build> {
    if (!this.appState.buildsService || !this.builder || this.buildNumOrId === undefined) {
      // Returns a promise that never resolves when the dependencies aren't
      // ready.
      return fromPromise(Promise.race([]));
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
    if (this.build$.state !== FULFILLED) {
      return null;
    }
    return new BuildExt(this.build$.value);
  }

  @computed
  private get relatedBuildReq(): IPromiseBasedObservable<readonly Build[]> {
    if (!this.build) {
      return fromPromise(Promise.race([]));
    }

    const buildsPromises = this.build.buildSets
      // Remove the commit/git/ buildsets because we know they're redundant with
      // the commit/gitiles/ buildsets, and we don't need to ask Buildbucket
      // twice.
      .filter((b) => !b.startsWith('commit/git/'))
      .map((b) => this.appState.buildsService!.searchBuilds({
        predicate: {tags: [{key: 'buildset', value: b}]},
        fields: '*',
        pageSize: 1000,
      }).then((res) => res.builds));

    return fromPromise(Promise.all(buildsPromises).then((buildArrays) => {
      const buildMap = new Map<string, Build>();
      for (const builds of buildArrays) {
        for (const build of builds){
          // Filter out duplicate builds by overwriting them.
          buildMap.set(build.id, build);
        }
      }
      return [...buildMap.values()]
        .sort((b1, b2) => {
          if (b1.id.length === b2.id.length) {
            return b1.id.localeCompare(b2.id);
          }
          return b1.id.length - b2.id.length;
        });
    }));
  }

  @computed({keepAlive: true})
  get relatedBuilds(): readonly BuildExt[] | null {
    if (this.isDisposed || this.relatedBuildReq.state !== FULFILLED) {
      return null;
    }
    return this.relatedBuildReq.value.map((build) => new BuildExt(build));
  }

  private getQueryBlamelistResIterFn(gitilesCommit: GitilesCommit, multiProjectSupport=false) {
    if (!this.appState.milo || !this.build) {
      return async function*() { await Promise.race([]); };
    }
    let req: QueryBlamelistRequest = {
      gitilesCommit,
      builder: this.build.builder,
      multiProjectSupport,
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

  @computed
  private get inputCommitRepo() {
    if (!this.build?.input.gitilesCommit) {
      return null;
    }
    return getGitilesRepoURL(this.build.input.gitilesCommit);
  }

  @computed({keepAlive: true})
  get queryBlamelistResIterFns() {
    if (this.isDisposed || !this.build) {
      return [];
    }

    return this.build.blamelistPins.map((pin) => {
      const pinRepo = getGitilesRepoURL(pin);
      return this.getQueryBlamelistResIterFn(pin, pinRepo !== this.inputCommitRepo);
    });
  }

  // Refresh all data that depends on the timestamp.
  refresh() {
    this.timestamp = Date.now();
  }
}

export const consumeBuildState = consumeContext<'buildState', BuildState>('buildState');
export const provideBuildState = provideContext<'buildState', BuildState>('buildState');
