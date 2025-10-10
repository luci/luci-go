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

import { useQuery, UseQueryResult } from '@tanstack/react-query';
import { useMemo } from 'react';

import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { getFilters } from '@/fleet/components/filter_dropdown/search_param_utils';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Column,
  ExportDevicesToCSVRequest,
  ExportDevicesToCSVResponse,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const useExportData = (
  columnsToExport: Column[],
  idsToExport?: string[],
) => {
  const [searchParams] = useSyncedSearchParams();
  const client = useFleetConsoleClient();
  const [orderByParam] = useOrderByParam();

  const selectedOptions = useMemo(
    () => getFilters(searchParams),
    [searchParams],
  );

  const stringifiedSelectedOptions = selectedOptions.error
    ? ''
    : stringifyFilters(selectedOptions.filters);

  const request = client.ExportDevicesToCSV.query(
    ExportDevicesToCSVRequest.fromPartial({
      columns: columnsToExport,
      orderBy: orderByParam,
      filter: stringifiedSelectedOptions,
      ids: idsToExport, // Export all if not provided
    }),
  );
  const query: UseQueryResult<ExportDevicesToCSVResponse, unknown> = useQuery({
    queryKey: request.queryKey,
    queryFn: request.queryFn,
    enabled: false,
    retry: 1,
  });

  return query;
};
