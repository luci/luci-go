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

import { debounce } from 'lodash-es';
import { Duration } from 'luxon';
import {
  addDisposer,
  addMiddleware,
  applySnapshot,
  getEnv,
  getSnapshot,
  hasEnv,
  Instance,
  SnapshotIn,
  SnapshotOut,
  types,
} from 'mobx-state-tree';

import { logging } from '@/common/tools/logging';

import { BuildConfig } from './build_config';
import { TestsConfig } from './tests_config';

export const V2_CACHE_KEY = 'user-config-v2';
const DEFAULT_TRANSIENT_KEY_TTL = Duration.fromObject({ week: 4 }).toMillis();

export interface UserConfigEnv {
  readonly storage?: Storage;
  readonly transientKeysTTL?: number;
}

export const UserConfig = types
  .model('UserConfig', {
    id: types.optional(types.identifierNumber, () => Math.random()),
    build: types.optional(BuildConfig, {}),
    tests: types.optional(TestsConfig, {}),
  })
  .actions((self) => ({
    deleteStaleKeys(before: Date) {
      self.build.deleteStaleKeys(before);
    },
    restoreConfig(storage: Storage) {
      try {
        const snapshotStr = storage.getItem(V2_CACHE_KEY);
        if (snapshotStr === null) {
          return;
        }
        applySnapshot(self, { ...JSON.parse(snapshotStr), id: self.id });
      } catch (e) {
        logging.error(e);
        logging.warn(
          'encountered an error when restoring user configs from the cache, deleting it',
        );
        storage.removeItem(V2_CACHE_KEY);
      }
    },
    enableCaching() {
      const env: UserConfigEnv = hasEnv(self) ? getEnv(self) : {};
      const storage = env.storage || window.localStorage;
      const ttl = env.transientKeysTTL || DEFAULT_TRANSIENT_KEY_TTL;

      this.restoreConfig(storage);
      this.deleteStaleKeys(new Date(Date.now() - ttl));

      const persistConfig = debounce(
        () => storage.setItem(V2_CACHE_KEY, JSON.stringify(getSnapshot(self))),
        // Add a tiny delay so updates happened in the same event cycle will
        // only trigger one save event.
        1,
      );

      addDisposer(
        self,
        // We cannot use `onAction` because it will not intercept actions
        // initiated on the ancestor nodes, even if those actions calls the
        // actions on this node.
        // Use `addMiddleware` allows use to intercept any action acted on this
        // node (and its descendent). However, if the parent decided to modify
        // the child node without calling its action, we still can't intercept
        // it. Currently, there's no way around this.
        //
        // See https://github.com/mobxjs/mobx-state-tree/issues/1948
        addMiddleware(self, (call, next) => {
          next(call, (value) => {
            // persistConfig AFTER the action is applied.
            persistConfig();
            return value;
          });
        }),
      );
    },
  }));

export type UserConfigInstance = Instance<typeof UserConfig>;
export type UserConfigSnapshotIn = SnapshotIn<typeof UserConfig>;
export type UserConfigSnapshotOut = SnapshotOut<typeof UserConfig>;
