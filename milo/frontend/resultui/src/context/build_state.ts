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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { autorun, computed, observable } from 'mobx';
import { fromPromise, IPromiseBasedObservable } from 'mobx-utils';

import { getGitilesRepoURL, renderBugUrlTemplate } from '../libs/build_utils';
import { POTENTIALLY_EXPIRED } from '../libs/constants';
import { createContextLink } from '../libs/context';
import * as iter from '../libs/iter_utils';
import { attachTags, InnerTag, TAG_SOURCE } from '../libs/tag';
import { unwrapObservable } from '../libs/unwrap_observable';
import { BuildExt } from '../models/build_ext';
import {
  Build,
  BUILD_FIELD_MASK,
  BuilderID,
  BuilderItem,
  GetBuildRequest,
  GitilesCommit,
  SEARCH_BUILD_FIELD_MASK,
} from '../services/buildbucket';
import { Project, QueryBlamelistRequest, QueryBlamelistResponse } from '../services/milo_internal';
import { getInvIdFromBuildId, getInvIdFromBuildNum } from '../services/resultdb';
import { AppState } from './app_state';

export class GetBuildError extends Error implements InnerTag {
  readonly [TAG_SOURCE]: Error;

  constructor(source: Error) {
    super(source.message);
    this[TAG_SOURCE] = source;
  }
}

/**
 * Records state of a build.
 */
export class BuildState {
  @observable.ref builderIdParam?: BuilderID;
  @observable.ref buildNumOrIdParam?: string;

  /**
   * Indicates whether a computed invocation ID should be used.
   * Computed invocation ID may not work on older builds.
   */
  @observable.ref useComputedInvId = true;

  @computed get builderId() {
    return this.builderIdParam || this.build?.builder;
  }

  /**
   * buildNum is defined when this.buildNumOrId is defined and doesn't start
   * with 'b'.
   */
  @computed get buildNum() {
    return this.buildNumOrIdParam?.startsWith('b') === false ? Number(this.buildNumOrIdParam) : null;
  }

  /**
   * buildId is defined when this.buildNumOrId is defined and starts with 'b',
   * or we have a matching cached build ID in appState.
   */
  @computed get buildId() {
    const cached =
      this.builderIdParam && this.buildNum !== null
        ? this.appState.getBuildId(this.builderIdParam, this.buildNum)
        : null;
    return cached || (this.buildNumOrIdParam?.startsWith('b') ? this.buildNumOrIdParam.slice(1) : null);
  }

  @computed private get invocationId$(): IPromiseBasedObservable<string> {
    if (!this.useComputedInvId) {
      if (this.build === null) {
        return fromPromise(Promise.race([]));
      }
      const invIdFromBuild = this.build.infra?.resultdb?.invocation?.slice('invocations/'.length) || '';
      return fromPromise(Promise.resolve(invIdFromBuild));
    } else if (this.buildId) {
      // Favor ID over builder + number to ensure cache hit when the build page
      // is redirected from a short build link to a long build link.
      return fromPromise(Promise.resolve(getInvIdFromBuildId(this.buildId)));
    } else if (this.builderIdParam && this.buildNum) {
      return fromPromise(getInvIdFromBuildNum(this.builderIdParam, this.buildNum));
    } else {
      return fromPromise(Promise.race([]));
    }
  }

  @computed get invocationId() {
    return unwrapObservable(this.invocationId$, null);
  }

  private disposers: Array<() => void> = [];

  constructor(private appState: AppState) {
    this.disposers.push(
      autorun(() => {
        if (!this.build) {
          return;
        }

        // If the associated gitiles commit is in the blamelist pins, select it.
        // Otherwise, select the first blamelist pin.
        const buildInputCommitRepo = this.build.associatedGitilesCommit
          ? getGitilesRepoURL(this.build.associatedGitilesCommit)
          : null;
        let selectedBlamelistPinIndex =
          this.build.blamelistPins.findIndex((pin) => getGitilesRepoURL(pin) === buildInputCommitRepo) || 0;
        if (selectedBlamelistPinIndex === -1) {
          selectedBlamelistPinIndex = 0;
        }
        this.selectedBlamelistPinIndex = selectedBlamelistPinIndex;
      })
    );
  }

  @observable.ref private isDisposed = false;

  /**
   * Perform cleanup.
   * Must be called before the object is GCed.
   */
  dispose() {
    this.isDisposed = true;

    // Evaluates @computed({keepAlive: true}) properties after this.isDisposed
    // is set to true so they no longer subscribes to any external observable.
    this.build$;
    this.relatedBuilds$;
    this.queryBlamelistResIterFns;
    this.permittedActions$;
    this.projectCfg$;

    this.disposers.reverse().forEach((disposer) => disposer());
    this.disposers = [];
  }

  private buildQueryTime = this.appState.timestamp;
  @computed({ keepAlive: true })
  private get build$(): IPromiseBasedObservable<BuildExt> {
    if (
      this.isDisposed ||
      !this.appState.buildsService ||
      (!this.buildId && (!this.builderIdParam || !this.buildNum))
    ) {
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
    const cacheOpt = {
      acceptCache: this.buildQueryTime >= this.appState.timestamp,
    };
    this.buildQueryTime = this.appState.timestamp;

    // Favor ID over builder + number to ensure cache hit when the build page is
    // redirected from a short build link to a long build link.
    const req: GetBuildRequest = this.buildId
      ? { id: this.buildId, fields: BUILD_FIELD_MASK }
      : { builder: this.builderIdParam, buildNumber: this.buildNum!, fields: BUILD_FIELD_MASK };

    return fromPromise(
      this.appState.buildsService
        .getBuild(req, cacheOpt)
        .catch((e) => {
          if (e instanceof GrpcError && e.code === RpcCode.NOT_FOUND) {
            attachTags(e, POTENTIALLY_EXPIRED);
          }
          throw new GetBuildError(e);
        })
        .then((b) => new BuildExt(b))
    );
  }

  @computed
  get build(): BuildExt | null {
    return unwrapObservable(this.build$, null);
  }

  @computed({ keepAlive: true })
  private get relatedBuilds$(): IPromiseBasedObservable<readonly BuildExt[]> {
    if (this.isDisposed || !this.build) {
      return fromPromise(Promise.race([]));
    }

    const buildsPromises = this.build.buildSets
      // Remove the commit/git/ buildsets because we know they're redundant with
      // the commit/gitiles/ buildsets, and we don't need to ask Buildbucket
      // twice.
      .filter((b) => !b.startsWith('commit/git/'))
      .map((b) =>
        this.appState
          .buildsService!.searchBuilds({
            predicate: { tags: [{ key: 'buildset', value: b }] },
            fields: SEARCH_BUILD_FIELD_MASK,
            pageSize: 1000,
          })
          .then((res) => res.builds)
      );

    return fromPromise(
      Promise.all(buildsPromises).then((buildArrays) => {
        const buildMap = new Map<string, Build>();
        for (const builds of buildArrays) {
          for (const build of builds) {
            // Filter out duplicate builds by overwriting them.
            buildMap.set(build.id, build);
          }
        }
        return [...buildMap.values()]
          .sort((b1, b2) => (b1.id.length === b2.id.length ? b1.id.localeCompare(b2.id) : b1.id.length - b2.id.length))
          .map((b) => new BuildExt(b));
      })
    );
  }

  @computed
  get relatedBuilds(): readonly BuildExt[] | null {
    return unwrapObservable(this.relatedBuilds$, null);
  }

  @observable.ref selectedBlamelistPinIndex = 0;

  private getQueryBlamelistResIterFn(gitilesCommit: GitilesCommit, multiProjectSupport = false) {
    if (!this.appState.milo || !this.build) {
      // eslint-disable-next-line require-yield
      return async function* () {
        await Promise.race([]);
      };
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
        req = { ...req, pageToken: res.nextPageToken };
        yield res;
      } while (res.nextPageToken);
    }
    return iter.teeAsync(streamBlamelist());
  }

  @computed
  private get gitilesCommitRepo() {
    if (!this.build?.associatedGitilesCommit) {
      return null;
    }
    return getGitilesRepoURL(this.build.associatedGitilesCommit);
  }

  @computed({ keepAlive: true })
  get queryBlamelistResIterFns() {
    if (this.isDisposed || !this.build) {
      return [];
    }

    return this.build.blamelistPins.map((pin) => {
      const pinRepo = getGitilesRepoURL(pin);
      return this.getQueryBlamelistResIterFn(pin, pinRepo !== this.gitilesCommitRepo);
    });
  }

  @computed({ keepAlive: true })
  private get builder$() {
    // We should not merge this with the if statement below because no other
    // observables should be accessed when this.isDisposed is set to true.
    if (this.isDisposed) {
      return fromPromise(Promise.race([]));
    }

    if (!this.appState.buildersService || !this.builderId) {
      return fromPromise(Promise.race([]));
    }
    return fromPromise(this.appState.buildersService.getBuilder({ id: this.builderId }));
  }

  @computed
  get builder(): BuilderItem | null {
    return unwrapObservable(this.builder$, null);
  }

  @computed private get bucketResourceId() {
    if (!this.builderId) {
      return null;
    }
    return `luci.${this.builderId.project}.${this.builderId.bucket}`;
  }

  @computed({ keepAlive: true })
  private get permittedActions$() {
    if (this.isDisposed || !this.appState.accessService || !this.bucketResourceId) {
      // Returns a promise that never resolves when the dependencies aren't
      // ready.
      return fromPromise(Promise.race([]));
    }

    // Establish a dependency on the timestamp.
    this.appState.timestamp;

    return fromPromise(
      this.appState.accessService?.permittedActions({
        resourceKind: 'bucket',
        resourceIds: [this.bucketResourceId],
      })
    );
  }

  @computed
  get permittedActions(): Set<string> {
    const permittedActionRes = unwrapObservable(this.permittedActions$, null);
    return new Set(permittedActionRes?.permitted[this.bucketResourceId!].actions || []);
  }

  @computed({ keepAlive: true })
  private get projectCfg$() {
    if (this.isDisposed || !this.appState.milo || !this.builderId?.project) {
      // Returns a promise that never resolves when the dependencies aren't
      // ready.
      return fromPromise(Promise.race([]));
    }

    // Establishes a dependency on the timestamp.
    this.appState.timestamp;

    return fromPromise(
      this.appState.milo.getProjectCfg({
        project: this.builderId.project,
      })
    );
  }

  @computed
  private get projectCfg(): Project | null {
    return unwrapObservable(this.projectCfg$, null);
  }

  @computed
  get customBugLink(): string | null {
    if (!this.build || !this.projectCfg?.bugUrlTemplate) {
      return null;
    }

    return renderBugUrlTemplate(this.projectCfg.bugUrlTemplate, this.build);
  }
}

export const [provideBuildState, consumeBuildState] = createContextLink<BuildState>();
