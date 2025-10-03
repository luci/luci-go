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

import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister';
import { QueryClient } from '@tanstack/react-query';
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client';

import { getIndexedDBWrapper } from './indexed_db_wrapper';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: Infinity, gcTime: Infinity }, // Keep gcTime Infinity
  },
});

const idbPersister = createAsyncStoragePersister({
  storage: getIndexedDBWrapper(),
});

/*
 * Persists queries with the key `['persist-local-storage']` in IndexedDB.
 */
export function LocalStoragePersistClientProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{
        persister: idbPersister,
        dehydrateOptions: {
          shouldDehydrateQuery: (query) => {
            const ret =
              query.queryKey.includes('persist-local-storage') &&
              query.state.status === 'success';
            return ret;
          },
        },
      }}
    >
      {children}
    </PersistQueryClientProvider>
  );
}
