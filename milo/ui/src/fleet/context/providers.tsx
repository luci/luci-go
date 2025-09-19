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

import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister';
import { QueryClient } from '@tanstack/react-query';
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client';

const queryClient = new QueryClient();

/*
 * Persists queries with the key `['persist-local-storage']` in the local storage.
 */
export function LocalStoragePersistClientProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const localStoragePersister = createSyncStoragePersister({
    storage: window.localStorage,
  });

  return (
    <PersistQueryClientProvider
      client={queryClient}
      // Followed the TkDodo suggestion: https://github.com/TanStack/query/discussions/7131#discussioncomment-8824550
      persistOptions={{
        persister: localStoragePersister,
        dehydrateOptions: {
          shouldDehydrateQuery: (query) => {
            return (
              query.queryKey.includes('persist-local-storage') &&
              query.state.status === 'success'
            );
          },
        },
      }}
    >
      {children}
    </PersistQueryClientProvider>
  );
}
