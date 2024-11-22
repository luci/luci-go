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
import * as React from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  ListDevicesRequest,
  Device,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { DataTable, generateColDefs } from '../data_table';

import { BASE_DIMENSIONS } from './columns';

const PAGE_SIZE = 25;

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
  const [nextPageTokens, setNextPageTokens] = React.useState<{
    [page: number]: string;
  }>({});
  const [paginationModel, setPaginationModel] = React.useState({
    page: 0,
    pageSize: PAGE_SIZE,
  });

  const client = useFleetConsoleClient();
  const { data, isLoading, isError, error } = useQuery(
    client.ListDevices.query(
      ListDevicesRequest.fromPartial({
        pageSize: paginationModel.pageSize,
        pageToken: nextPageTokens[paginationModel.page - 1],
      }),
    ),
  );

  if (isError) {
    throw error;
  }

  const { devices = [], nextPageToken = '' } = data || {};
  const { columns, rows } = processDeviceData({ devices });

  React.useEffect(() => {
    if (nextPageToken) {
      setNextPageTokens((prev) => ({
        ...prev,
        [paginationModel.page]: nextPageToken,
      }));
    }
  }, [paginationModel.page, nextPageToken, setNextPageTokens]);

  return (
    <DataTable
      nextPageToken={nextPageToken}
      isLoading={isLoading}
      paginationModel={paginationModel}
      onPaginationModelChange={(model) => setPaginationModel(model)}
      columns={generateColDefs(columns)}
      rows={rows}
    />
  );
}
