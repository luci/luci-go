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

import { getGitilesRepoURL } from '../libs/build_utils';
import { CacheOption } from '../libs/cached_fn';
import { consumeContext, provideContext } from '../libs/context';
import * as iter from '../libs/iter_utils';
import { BuildExt } from '../models/build_ext';
import { Build, BuilderID, BuilderItem, GetBuildRequest, GitilesCommit } from '../services/buildbucket';
import { QueryBlamelistRequest, QueryBlamelistResponse } from '../services/milo_internal';
import { getInvIdFromBuildId, getInvIdFromBuildNum } from '../services/resultdb';
import { AppState } from './app_state';

/**
 * Records state of a build.
 */
export class BuildState {
  @observable.ref builderId?: BuilderID;
  @observable.ref buildNumOrId?: string;

  /**
   * Indicates whether a computed invocation ID should be used.
   * Computed invocation ID may not work on older builds.
   */
  @observable.ref useComputedInvId = true;

  /**
   * buildNum is defined when this.buildNumOrId is defined and doesn't start
   * with 'b'.
   */
  @computed get buildNum() {
    return this.buildNumOrId?.startsWith('b') === false ? Number(this.buildNumOrId) : null;
  }

  /**
   * buildId is defined when this.buildNumOrId is defined and starts with 'b'.
   */
  @computed get buildId() {
    return this.buildNumOrId?.startsWith('b') ? this.buildNumOrId.slice(1) : null;
  }

  @computed private get invocationId$(): IPromiseBasedObservable<string> {
    if (!this.useComputedInvId) {
      if (this.build === null) {
        return fromPromise(Promise.race([]));
      }
      const invIdFromBuild = this.build.infra?.resultdb?.invocation?.slice('invocations/'.length) || '';
      return fromPromise(Promise.resolve(invIdFromBuild));
    } else if (this.builderId && this.buildNum) {
      return fromPromise(getInvIdFromBuildNum(this.builderId, this.buildNum));
    } else if (this.buildId) {
      return fromPromise(Promise.resolve(getInvIdFromBuildId(this.buildId)));
    } else {
      return fromPromise(Promise.race([]));
    }
  }

  @computed get invocationId() {
    return this.invocationId$.state === FULFILLED ? this.invocationId$.value : null;
  }

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

  private buildQueryTime = 0;
  @computed
  get build$(): IPromiseBasedObservable<Build> {
    if (!this.appState.buildsService || (!this.buildId && (!this.builderId || !this.buildNum))) {
      // Returns a promise that never resolves when the dependencies aren't
      // ready.
      return fromPromise(Promise.race([]));
    }

    // If we use a simple boolean property here,
    // 1. the boolean property cannot be an observable because we don't want to
    // update observables in a computed property, and
    // 2. we still need an observable (like this.timestamp) to trigger the
    // update, and
    // 3. this.refresh() will need to reset the boolean properties of all
    // time-sensitive computed value.
    //
    // If we record the query time instead, no other code will need to read
    // or update the query time.
    const cacheOpt = this.buildQueryTime < this.timestamp
      ? CacheOption.ForceRefresh
      : CacheOption.Cached;
    this.buildQueryTime = this.timestamp;

    const req: GetBuildRequest = this.buildId
      ? {id: this.buildId, fields: '*'}
      : {builder: this.builderId, buildNumber: this.buildNum!, fields: '*'};

    return fromPromise(this.appState.buildsService.getBuild(req, cacheOpt));
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

  @computed({keepAlive: true})
  private get builder$() {
    // We should not merge this with the if statement below because no other
    // observables should be accessed when this.isDisposed is set to true.
    if (this.isDisposed) {
      return fromPromise(Promise.race([]));
    }

    const builderId = this.builderId || this.build?.builder;
    if (!this.appState.buildersService || !builderId) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.buildersService.getBuilder({id: builderId}));
  }

  @computed
  get builder(): BuilderItem | null {
    return this.builder$.state === FULFILLED ? this.builder$.value : null;
  }

  // Refresh all data that depends on the timestamp.
  refresh() {
    this.timestamp = Date.now();
  }
}

export const consumeBuildState = consumeContext<'buildState', BuildState>('buildState');
export const provideBuildState = provideContext<'buildState', BuildState>('buildState');
