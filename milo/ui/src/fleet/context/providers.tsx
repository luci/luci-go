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
    queries: {
      staleTime: 0,
      gcTime: 0, // Disable caching by default by making gcTime 0.
    },
  },
});

const idbPersister = createAsyncStoragePersister({
  storage: getIndexedDBWrapper(),
});

export function IndexedDBPersistClientProvider({
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
            return query.state.status === 'success';
          },
        },
      }}
    >
      {children}
    </PersistQueryClientProvider>
  );
}
