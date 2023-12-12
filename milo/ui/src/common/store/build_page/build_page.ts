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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import stableStringify from 'fast-json-stable-stringify';
import { reaction } from 'mobx';
import {
  addDisposer,
  cast,
  Instance,
  SnapshotIn,
  SnapshotOut,
  types,
} from 'mobx-state-tree';
import { fromPromise } from 'mobx-utils';

import {
  NEVER_OBSERVABLE,
  POTENTIALLY_EXPIRED,
} from '@/common/constants/legacy';
import {
  Build,
  BUILD_FIELD_MASK,
  BuilderID,
  GetBuildRequest,
  PERM_BUILDS_ADD,
  PERM_BUILDS_CANCEL,
  PERM_BUILDS_GET,
  PERM_BUILDS_GET_LIMITED,
  TEST_PRESENTATION_KEY,
  Trinary,
} from '@/common/services/buildbucket';
import {
  getInvIdFromBuildId,
  getInvIdFromBuildNum,
  PERM_INVOCATIONS_GET,
  PERM_TEST_EXONERATIONS_LIST,
  PERM_TEST_EXONERATIONS_LIST_LIMITED,
  PERM_TEST_RESULTS_LIST,
  PERM_TEST_RESULTS_LIST_LIMITED,
} from '@/common/services/resultdb';
import { BuildState } from '@/common/store/build_state';
import { InvocationState } from '@/common/store/invocation_state';
import { ServicesStore } from '@/common/store/services';
import { Timestamp } from '@/common/store/timestamp';
import { UserConfig } from '@/common/store/user_config';
import { getGitilesRepoURL } from '@/common/tools/gitiles_utils';
import {
  aliveFlow,
  keepAliveComputed,
  unwrapObservable,
} from '@/generic_libs/tools/mobx_utils';
import { attachTags, InnerTag, TAG_SOURCE } from '@/generic_libs/tools/tag';

export const enum SearchTarget {
  Builders,
  Tests,
}

export class GetBuildError extends Error implements InnerTag {
  readonly [TAG_SOURCE]: Error;

  constructor(source: Error) {
    super(source.message);
    this[TAG_SOURCE] = source;
  }
}

export const BuildPage = types
  .model('BuildPage', {
    currentTime: types.safeReference(Timestamp),
    refreshTime: types.safeReference(Timestamp),
    services: types.safeReference(ServicesStore),
    userConfig: types.safeReference(UserConfig),

    /**
     * The builder ID of the build.
     * Ignored when build `buildNumOrIdParam` is a build ID string (i.e. begins
     * with 'b').
     */
    builderIdParam: types.maybe(types.frozen<BuilderID>()),
    buildNumOrIdParam: types.maybe(types.string),

    /**
     * Indicates whether a computed invocation ID should be used.
     * Computed invocation ID may not work on older builds.
     */
    useComputedInvId: true,
    invocation: types.optional(InvocationState, {}),

    // Properties that provide a mounting point for computed models so they can
    // have references to some other properties in the tree.
    _build: types.maybe(BuildState),
  })
  .volatile(() => {
    const cachedBuildId = new Map<string, string>();
    return {
      setBuildId(builderId: BuilderID, buildNum: number, buildId: string) {
        cachedBuildId.set(stableStringify([builderId, buildNum]), buildId);
      },
      getBuildId(builderId: BuilderID, buildNum: number) {
        return cachedBuildId.get(stableStringify([builderId, buildNum]));
      },
    };
  })
  .views((self) => ({
    /**
     * buildNum is defined when this.buildNumOrId is defined and doesn't start
     * with 'b'.
     */
    get buildNum() {
      return self.buildNumOrIdParam?.startsWith('b') === false
        ? Number(self.buildNumOrIdParam)
        : null;
    },
    /**
     * buildId is defined when this.buildNumOrId is defined and starts with 'b',
     * or we have a matching cached build ID in appState.
     */
    get buildId() {
      const cached =
        self.builderIdParam && this.buildNum !== null
          ? self.getBuildId(self.builderIdParam, this.buildNum)
          : null;
      return (
        cached ||
        (self.buildNumOrIdParam?.startsWith('b')
          ? self.buildNumOrIdParam.slice(1)
          : null)
      );
    },
    get hasInvocation() {
      return Boolean(self._build?.data.infra?.resultdb?.invocation);
    },
  }))
  .actions((self) => ({
    _setBuild(build: Build) {
      self._build = cast({
        data: build,
        currentTime: self.currentTime?.id,
        userConfig: self.userConfig?.id,
      });
    },
  }))
  .views((self) => {
    let buildQueryTime: number | null = null;
    const build = keepAliveComputed(self, () => {
      if (
        !self.services?.builds ||
        (!self.buildId && (!self.builderIdParam || !self.buildNum)) ||
        !self.refreshTime
      ) {
        return null;
      }

      // If we use a simple boolean property here,
      // 1. the boolean property cannot be an observable because we don't want
      // to update observables in a computed property, and
      // 2. we still need an observable (like this.timestamp) to trigger the
      // update, and
      // 3. this.refresh() will need to reset the boolean properties of all
      // time-sensitive computed value.
      //
      // If we record the query time instead, no other code will need to read
      // or update the query time.
      const cacheOpt = {
        acceptCache:
          buildQueryTime === null || buildQueryTime >= self.refreshTime.value,
      };
      buildQueryTime = self.refreshTime.value;

      // Favor ID over builder + number to ensure cache hit when the build
      // page is redirected from a short build link to a long build link.
      const req: GetBuildRequest = self.buildId
        ? { id: self.buildId, fields: BUILD_FIELD_MASK }
        : {
            builder: self.builderIdParam,
            buildNumber: self.buildNum!,
            fields: BUILD_FIELD_MASK,
          };

      return fromPromise(
        self.services.builds
          .getBuild(req, cacheOpt)
          .catch((e) => {
            if (e instanceof GrpcError && e.code === RpcCode.NOT_FOUND) {
              attachTags(e, POTENTIALLY_EXPIRED);
            }
            throw new GetBuildError(e);
          })
          .then((b) => {
            self._setBuild(b);
            return self._build!;
          }),
      );
    });
    return {
      get build() {
        return unwrapObservable(build.get() || NEVER_OBSERVABLE, null);
      },
    };
  })
  .views((self) => {
    const invocationId = keepAliveComputed(self, () => {
      if (!self.useComputedInvId) {
        if (self.build === null) {
          return null;
        }
        const invIdFromBuild =
          self.build?.data.infra?.resultdb?.invocation?.slice(
            'invocations/'.length,
          ) ?? null;
        return fromPromise(Promise.resolve(invIdFromBuild));
      } else if (self.buildId) {
        // Favor ID over builder + number to ensure cache hit when the build
        // page is redirected from a short build link to a long build link.
        return fromPromise(Promise.resolve(getInvIdFromBuildId(self.buildId)));
      } else if (self.builderIdParam && self.buildNum) {
        return fromPromise(
          getInvIdFromBuildNum(self.builderIdParam, self.buildNum),
        );
      } else {
        return null;
      }
    });

    const permittedActions = keepAliveComputed(self, () => {
      if (!self.services?.milo || !self.build?.data.builder) {
        return null;
      }

      // Establish a dependency on the timestamp.
      self.refreshTime?.value;

      return fromPromise(
        self.services.milo.batchCheckPermissions({
          realm: `${self.build.data.builder.project}:${self.build.data.builder.bucket}`,
          permissions: [
            PERM_BUILDS_CANCEL,
            PERM_BUILDS_ADD,
            PERM_BUILDS_GET,
            PERM_BUILDS_GET_LIMITED,
            PERM_INVOCATIONS_GET,
            PERM_TEST_EXONERATIONS_LIST,
            PERM_TEST_RESULTS_LIST,
            PERM_TEST_EXONERATIONS_LIST_LIMITED,
            PERM_TEST_RESULTS_LIST_LIMITED,
          ],
        }),
      );
    });

    return {
      get invocationId() {
        return unwrapObservable(invocationId.get() || NEVER_OBSERVABLE, null);
      },
      get _permittedActions(): { readonly [key: string]: boolean | undefined } {
        const permittedActionRes = unwrapObservable(
          permittedActions.get() || NEVER_OBSERVABLE,
          null,
        );
        return permittedActionRes?.results || {};
      },
      get canRetry() {
        return Boolean(
          self.build?.data.retriable !== Trinary.No &&
            this._permittedActions[PERM_BUILDS_ADD],
        );
      },
      get canCancel() {
        return this._permittedActions[PERM_BUILDS_CANCEL] || false;
      },
      get canReadFullBuild() {
        return this._permittedActions[PERM_BUILDS_GET] || false;
      },
      get canReadTestVerdicts() {
        return (
          this._permittedActions[PERM_TEST_EXONERATIONS_LIST_LIMITED] &&
          this._permittedActions[PERM_TEST_RESULTS_LIST_LIMITED]
        );
      },
      get gitilesCommitRepo() {
        if (!self.build?.associatedGitilesCommit) {
          return null;
        }
        return getGitilesRepoURL(self.build.associatedGitilesCommit);
      },
    };
  })
  .actions((self) => ({
    setDependencies(
      deps: Partial<
        Pick<
          typeof self,
          'currentTime' | 'refreshTime' | 'services' | 'userConfig'
        >
      >,
    ) {
      Object.assign<typeof self, Partial<typeof self>>(self, deps);
    },
    setUseComputedInvId(useComputed: boolean) {
      self.useComputedInvId = useComputed;
    },
    setParams(builderId: BuilderID | undefined, buildNumOrId: string) {
      self.builderIdParam = builderId;
      self.buildNumOrIdParam = buildNumOrId;
    },
    retryBuild: aliveFlow(self, function* () {
      if (!self.build?.data.id || !self.services?.builds) {
        return null;
      }

      const call = self.services.builds.scheduleBuild({
        templateBuildId: self.build.data.id,
      });
      const build: Awaited<typeof call> = yield call;
      return build;
    }),
    cancelBuild: aliveFlow(self, function* (reason: string) {
      if (!self.build?.data.id || !reason || !self.services?.builds) {
        return;
      }

      yield self.services.builds.cancelBuild({
        id: self.build.data.id,
        summaryMarkdown: reason,
      });
      self.refreshTime?.refresh();
    }),
    afterCreate() {
      addDisposer(
        self,
        reaction(
          () => self.services,
          (services) => {
            self.invocation.setDependencies({
              services,
            });
          },
          { fireImmediately: true },
        ),
      );

      self.invocation.setDependencies({
        invocationIdGetter: () => self.invocationId,
        presentationConfigGetter: () =>
          self.build?.data.output?.properties?.[TEST_PRESENTATION_KEY] ||
          self.build?.data.input?.properties?.[TEST_PRESENTATION_KEY] ||
          {},
        warningGetter: () =>
          self.build?.buildOrStepInfraFailed
            ? 'Test results displayed here are likely incomplete because some steps have infra failed.'
            : '',
      });
    },
  }));

export type BuildPageInstance = Instance<typeof BuildPage>;
export type BuildPageSnapshotIn = SnapshotIn<typeof BuildPage>;
export type BuildPageSnapshotOut = SnapshotOut<typeof BuildPage>;
