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

import { PrpcClientOptions } from '@chopsui/prpc-client';
import { computed, untracked } from 'mobx';
import {
  addDisposer,
  Instance,
  isAlive,
  SnapshotIn,
  SnapshotOut,
  types,
} from 'mobx-state-tree';
import { keepAlive } from 'mobx-utils';

import { MAY_REQUIRE_SIGNIN } from '@/common/common_tags';
import { POTENTIAL_PERM_ERROR_CODES } from '@/common/constants/rpc';
import {
  ClustersService,
  TestHistoryService,
} from '@/common/services/luci_analysis';
import { ResultDb } from '@/common/services/resultdb';
import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';
import { attachTags } from '@/generic_libs/tools/tag';

import { AuthStateStore, AuthStateStoreInstance } from './auth_state';

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
        () =>
          untracked(
            () => (isAlive(self) && self.authState?.value?.accessToken) || '',
          ),
        (e) => {
          if (POTENTIAL_PERM_ERROR_CODES.includes(e.code)) {
            attachTags(e, MAY_REQUIRE_SIGNIN);
          }
          throw e;
        },
      );
    }

    return {
      get resultDb() {
        if (!self.authState?.identity) {
          return null;
        }
        return new ResultDb(makeClient({ host: SETTINGS.resultdb.host }));
      },
      get testHistory() {
        if (!self.authState?.identity) {
          return null;
        }
        return new TestHistoryService(
          makeClient({ host: SETTINGS.luciAnalysis.host }),
        );
      },
      get clusters() {
        if (!self.authState?.identity) {
          return null;
        }
        return new ClustersService(
          makeClient({ host: SETTINGS.luciAnalysis.host }),
        );
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
      addDisposer(self, keepAlive(computed(() => self.clusters)));
    },
  }));

export type ServicesStoreInstance = Instance<typeof ServicesStore>;
export type ServicesStoreSnapshotIn = SnapshotIn<typeof ServicesStore>;
export type ServicesStoreSnapshotOut = SnapshotOut<typeof ServicesStore>;
