// Copyright 2024 The LUCI Authors.
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

import {
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ListDevicesRequest,
  Device,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { DataTable, generateColDefs } from '../data_table';

import { BASE_DIMENSIONS } from './columns';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

function processDeviceData({ devices }: { devices: readonly Device[] }): {
  columns: string[];
  rows: Record<string, string>[];
} {
  const rows: Record<string, string>[] = [];
  const columns: string[] = [
    'id',
    'dutId',
    'dutType',
    'dutState',
    'dutHost',
    'dutPort',
  ];

  devices.forEach((device) => {
    const row: Record<string, string> = Object.fromEntries(
      Object.entries(BASE_DIMENSIONS).map(([key, fn]) => [key, fn(device)]),
    );

    if (device.deviceSpec) {
      device.deviceSpec.labels;
      for (const label of Object.keys(device.deviceSpec.labels)) {
        // TODO(b/378634266): should be discussed how to show multiple values
        row[label] = device.deviceSpec.labels[label].values[0];
        // TODO(vaghinak): later this will be replaced with the GetDimensions api
        if (!columns.includes(label)) {
          columns.push(label);
        }
      }
    }

    rows.push(row);
  });

  return { columns, rows };
}

export function DeviceTable() {
  const [searchParams] = useSyncedSearchParams();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const client = useFleetConsoleClient();
  const { data, isLoading, isError, error } = useQuery(
    client.ListDevices.query(
      ListDevicesRequest.fromPartial({
        pageSize: getPageSize(pagerCtx, searchParams),
        pageToken: getPageToken(pagerCtx, searchParams),
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  const { devices = [], nextPageToken = '' } = data || {};
  const { columns, rows } = processDeviceData({ devices });

  return (
    <DataTable
      nextPageToken={nextPageToken}
      isLoading={isLoading}
      pagerCtx={pagerCtx}
      columns={generateColDefs(columns)}
      rows={rows}
    />
  );
}
