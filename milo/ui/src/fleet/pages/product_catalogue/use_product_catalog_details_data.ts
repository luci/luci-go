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

import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { formatAipClause } from '@/fleet/utils/search_param';

import {
  UnifiedProductCatalogEntry,
  fromStandardCatalogEntry,
  fromGceCatalogEntry,
} from './types';

export type UseProductCatalogDetailsDataResult = {
  error?: unknown;
  isError: boolean;
  isLoading: boolean;
  entry?: UnifiedProductCatalogEntry;
};

/**
 * Queries for a product catalog entry using ListProductCatalogEntries query
 * or ListGceProductCatalogEntries query with a single product_catalog_id filter.
 *
 * @param id - the product catalog id to query
 */
export const useProductCatalogDetailsData = (
  id: string,
): UseProductCatalogDetailsDataResult => {
  const client = useFleetConsoleClient();

  const filter = formatAipClause('product_catalog_id', [id]);

  const standardQuery = useQuery({
    ...client.ListProductCatalogEntries.query({ filter }),
    enabled: !!id,
  });

  const gceQuery = useQuery({
    ...client.ListGceProductCatalogEntries.query({ filter }),
    enabled: !!id,
  });

  const entry = useMemo(() => {
    if (standardQuery.data?.entries?.[0]) {
      return fromStandardCatalogEntry(standardQuery.data.entries[0]);
    }
    if (gceQuery.data?.entries?.[0]) {
      return fromGceCatalogEntry(gceQuery.data.entries[0]);
    }
    return undefined;
  }, [standardQuery.data, gceQuery.data]);

  const hasEntry = !!entry;

  return {
    error: standardQuery.error || gceQuery.error,
    isError: !hasEntry && Boolean(standardQuery.isError || gceQuery.isError),
    isLoading: !hasEntry && (standardQuery.isLoading || gceQuery.isLoading),
    entry,
  };
};
