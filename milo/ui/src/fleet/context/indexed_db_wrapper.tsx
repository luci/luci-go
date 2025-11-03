// Copyright 2025 The LUCI Authors.
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

import {
  PersistedClient,
  Persister,
} from '@tanstack/react-query-persist-client';
import { clear, createStore, get, keys, set } from 'idb-keyval';

import { DEFAULT_IDB_STORE_NAME } from '../constants/caching_keys';

/**
 * This persister stores metadata separately from the queries themselves.
 * This allows for more efficient garbage collection and restoration,
 * addressing performance concerns with large caches.
 */
const CLIENT_METADATA_KEY = 'CLIENT_METADATA';

/**
 * Creates an Indexed DB persister
 *
 * This persister stores each query key in its own IDB store key to minimize
 * issues with data merging while also allowing for performant storage and
 * retrieval. (ie: we don't need to load data for queries that are not in use.)
 *
 * @see https://developer.mozilla.org/en-US/docs/Web/API/IndexedDB_API
 * @see https://tanstack.com/query/v4/docs/framework/react/plugins/persistQueryClient#building-a-persister
 */
export function createIDBPersister() {
  const customStore = createStore(DEFAULT_IDB_STORE_NAME, 'queries');
  return {
    persistClient: async (client: PersistedClient) => {
      const metadata: Omit<PersistedClient, 'clientState'> = {
        buster: client.buster,
        timestamp: client.timestamp,
      };
      await set(CLIENT_METADATA_KEY, metadata, customStore);

      const newQueryHashes = new Set<string>();
      for (const query of client.clientState.queries) {
        newQueryHashes.add(query.queryHash);
        await set(query.queryHash, query, customStore);
      }
    },
    restoreClient: async (): Promise<PersistedClient | undefined> => {
      const metadata = await get<Omit<PersistedClient, 'clientState'>>(
        CLIENT_METADATA_KEY,
        customStore,
      );

      if (!metadata) {
        return undefined;
      }

      const allKeys = await keys(customStore);
      const queryKeys = allKeys.filter((key) => key !== CLIENT_METADATA_KEY);
      const queries = (
        await Promise.all(queryKeys.map((key) => get(key, customStore)))
      ).filter(Boolean);

      return {
        ...metadata,
        clientState: {
          mutations: [],
          queries,
        },
      };
    },
    removeClient: async () => {
      await clear(customStore);
    },
  } satisfies Persister;
}
