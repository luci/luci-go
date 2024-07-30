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

import { NEVER_OBSERVABLE } from '@/common/constants/legacy';
import {
  Build,
  BuilderID,
  TEST_PRESENTATION_KEY,
} from '@/common/services/buildbucket';
import {
  getInvIdFromBuildId,
  getInvIdFromBuildNum,
} from '@/common/services/resultdb';
import { BuildState } from '@/common/store/build_state';
import { InvocationState } from '@/common/store/invocation_state';
import { ServicesStore } from '@/common/store/services';
import { Timestamp } from '@/common/store/timestamp';
import { UserConfig } from '@/common/store/user_config';
import {
  keepAliveComputed,
  unwrapObservable,
} from '@/generic_libs/tools/mobx_utils';
import { InnerTag, TAG_SOURCE } from '@/generic_libs/tools/tag';

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
    _buildState: types.maybe(BuildState),
  })
  .views((self) => ({
    /**
     * buildNumFromParam is defined when this.buildNumOrId is defined and
     * doesn't start with 'b'.
     */
    get buildNumFromParam() {
      return self.buildNumOrIdParam?.startsWith('b') === false
        ? Number(self.buildNumOrIdParam)
        : null;
    },
    /**
     * buildIdFromParam is defined when this.buildNumOrId is defined and starts
     * with 'b'.
     */
    get buildIdFromParam() {
      return self.buildNumOrIdParam?.startsWith('b')
        ? self.buildNumOrIdParam.slice(1)
        : null;
    },
    get hasInvocation() {
      return Boolean(self._buildState?.data.infra?.resultdb?.invocation);
    },
  }))
  .actions((self) => ({
    setBuild(build: Build | null) {
      if (build === null) {
        self._buildState = undefined;
        return;
      }
      self._buildState = cast({
        data: build,
        currentTime: self.currentTime?.id,
        userConfig: self.userConfig?.id,
      });
    },
  }))
  .views((self) => {
    return {
      get build() {
        return self._buildState || null;
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
      } else if (self.buildIdFromParam) {
        return fromPromise(
          Promise.resolve(getInvIdFromBuildId(self.buildIdFromParam)),
        );
      } else if (self.builderIdParam && self.buildNumFromParam) {
        return fromPromise(
          getInvIdFromBuildNum(self.builderIdParam, self.buildNumFromParam),
        );
      } else {
        return null;
      }
    });

    return {
      get invocationId() {
        return unwrapObservable(invocationId.get() || NEVER_OBSERVABLE, null);
      },
    };
  })
  .actions((self) => ({
    setDependencies(
      deps: Partial<
        Pick<typeof self, 'currentTime' | 'services' | 'userConfig'>
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
