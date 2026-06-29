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

import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export type UseProductCatalogDetailsDataResult = {
  error?: unknown;
  isError: boolean;
  isLoading: boolean;
  entry?: ProductCatalogEntry;
};

/**
 * Queries for a product catalog entry using ListProductCatalogEntries query
 * with a single product_catalog_id filter.
 *
 * @param id - the product catalog id to query
 */
export const useProductCatalogDetailsData = (
  id: string,
): UseProductCatalogDetailsDataResult => {
  const client = useFleetConsoleClient();

  const filter = stringifyFilters({ product_catalog_id: [id] });

  const { data, error, isError, isLoading } = useQuery({
    ...client.ListProductCatalogEntries.query({ filter }),
    enabled: !!id,
  });

  return {
    error,
    isError,
    isLoading,
    entry: data?.entries?.[0] ?? undefined,
  };
};
