// Copyright 2026 The LUCI Authors.
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

import { useEffect } from 'react';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { mapUrl } from './rri_url_migration';

export const useRriUrlMigration = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const { migratedParams, needsMigration } = mapUrl(searchParams);

  useEffect(() => {
    if (needsMigration) {
      setSearchParams(migratedParams, { replace: true });
    }
  }, [needsMigration, migratedParams, setSearchParams]);

  return { isMigrating: needsMigration };
};
