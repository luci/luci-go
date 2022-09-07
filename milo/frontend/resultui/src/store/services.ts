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

import { PrpcClientOptions, RpcCode } from '@chopsui/prpc-client';
import { computed, untracked } from 'mobx';
import { addDisposer, Instance, isAlive, SnapshotIn, SnapshotOut, types } from 'mobx-state-tree';
import { keepAlive } from 'mobx-utils';

import { MAY_REQUIRE_SIGNIN } from '../common_tags';
import { PrpcClientExt } from '../libs/prpc_client_ext';
import { attachTags } from '../libs/tag';
import { BuildersService, BuildsService } from '../services/buildbucket';
import { MiloInternal } from '../services/milo_internal';
import { ResultDb } from '../services/resultdb';
import { ClustersService, TestHistoryService } from '../services/weetbix';
import { AuthStateStore, AuthStateStoreInstance } from './auth_state';

const MAY_REQUIRE_SIGNIN_ERROR_CODE = [RpcCode.NOT_FOUND, RpcCode.PERMISSION_DENIED, RpcCode.UNAUTHENTICATED];

export const ServicesStore = types
  .model('ServicesStore', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    authState: types.safeReference(AuthStateStore),
  })
  .views((self) => {
    function makeClient(opts: PrpcClientOptions) {
      // Don't track the access token so services won't be refreshed when the
      // access token is updated.
      return new PrpcClientExt(
        opts,
        () => untracked(() => (isAlive(self) && self.authState?.value?.accessToken) || ''),
        (e) => {
          if (MAY_REQUIRE_SIGNIN_ERROR_CODE.includes(e.code)) {
            attachTags(e, MAY_REQUIRE_SIGNIN);
          }
          throw e;
        }
      );
    }

    return {
      get resultDb() {
        if (!self.authState?.userIdentity) {
          return null;
        }
        return new ResultDb(makeClient({ host: CONFIGS.RESULT_DB.HOST }));
      },
      get testHistory() {
        if (!self.authState?.userIdentity) {
          return null;
        }
        return new TestHistoryService(makeClient({ host: CONFIGS.WEETBIX.HOST }));
      },
      get milo() {
        if (!self.authState?.userIdentity) {
          return null;
        }
        return new MiloInternal(makeClient({ host: '' }));
      },
      get builds() {
        if (!self.authState?.userIdentity) {
          return null;
        }
        return new BuildsService(makeClient({ host: CONFIGS.BUILDBUCKET.HOST }));
      },
      get builders() {
        if (!self.authState?.userIdentity) {
          return null;
        }
        return new BuildersService(makeClient({ host: CONFIGS.BUILDBUCKET.HOST }));
      },
      get clusters() {
        if (!self.authState?.userIdentity) {
          return null;
        }
        return new ClustersService(makeClient({ host: CONFIGS.WEETBIX.HOST }));
      },
    };
  })
  .actions((self) => ({
    setDependencies({ authState }: { authState: AuthStateStoreInstance }) {
      self.authState = authState;
    },
    afterCreate() {
      // These computed properties contains internal caches. Keep them alive.
      addDisposer(self, keepAlive(computed(() => self.resultDb)));
      addDisposer(self, keepAlive(computed(() => self.testHistory)));
      addDisposer(self, keepAlive(computed(() => self.milo)));
      addDisposer(self, keepAlive(computed(() => self.builds)));
      addDisposer(self, keepAlive(computed(() => self.builders)));
      addDisposer(self, keepAlive(computed(() => self.clusters)));
    },
  }));

export type ServicesStoreInstance = Instance<typeof ServicesStore>;
export type ServicesStoreSnapshotIn = SnapshotIn<typeof ServicesStore>;
export type ServicesStoreSnapshotOut = SnapshotOut<typeof ServicesStore>;
