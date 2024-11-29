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

import { GrpcError } from '@chopsui/prpc-client';
import { Alert } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import {
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Device,
  ListDevicesRequest,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { DataTable, generateColDefs } from '../data_table';

import { GridColDef, GridSortModel } from '@mui/x-data-grid';
import { useState } from 'react';
import { BASE_DIMENSIONS } from './columns';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

function getErrorMessage(error: unknown): string {
  if (error instanceof GrpcError) {
    if (error.code === 7) {
      return 'You dont have permission to list devices';
    }

    return error.description;
  }
  return 'Unknown error';
}

function processDeviceData({ devices }: { devices: readonly Device[] }): {
  columns: GridColDef[];
  rows: Record<string, string>[];
} {
  const rows: Record<string, string>[] = [];
  const columns: { [key: string]: string } = BASE_DIMENSIONS.reduce((acc: { [key: string]: string }, dim) => {
    acc[dim.id] = dim.displayName;
    return acc;
  }, {});

  devices.forEach((device) => {
    const row: Record<string, string> = Object.fromEntries(
      BASE_DIMENSIONS.map((dim) => [dim.id, dim.getValue(device)]),
    );

    if (device.deviceSpec) {
      for (const label of Object.keys(device.deviceSpec.labels)) {
        // TODO(b/378634266): should be discussed how to show multiple values
        row[label] = device.deviceSpec.labels[label].values.concat().sort((a, b) => a.length < b.length ? -1 : 1)[0].toString();
        // TODO(vaghinak): later this will be replaced with the GetDimensions api
        if (!columns[label]) {
          columns[label] = label;
        }
      }
    }

    rows.push(row);
  });

  return { columns: generateColDefs(columns), rows };
}

export function DeviceTable() {
  const [searchParams] = useSyncedSearchParams();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });
  const [sortModel, setSortModel] = useState<GridSortModel>([])

  const getOrderByFromSortModel = () => {
    if (sortModel.length !== 1) {
      return '';
    }
    const sortItem = sortModel[0];
    const baseDimension = BASE_DIMENSIONS.filter(dim => dim.id === sortItem.field)[0]

    const sortKey = baseDimension ? baseDimension.id : `labels.${sortItem.field}`

    return sortItem.sort === 'desc' ? `${sortKey} desc` : sortKey
  };

  const client = useFleetConsoleClient();
  const { data, isLoading, isError, error } = useQuery(
    client.ListDevices.query(
      ListDevicesRequest.fromPartial({
        pageSize: getPageSize(pagerCtx, searchParams),
        pageToken: getPageToken(pagerCtx, searchParams),
        orderBy: getOrderByFromSortModel()
      }),
    ),
  );

  const { devices = [], nextPageToken = '' } = data || {};
  const { columns, rows } = processDeviceData({ devices });

  return (
    <>
      {isError ? (
        <Alert severity="error">
          {' '}
          Something went wrong: {getErrorMessage(error)}
        </Alert>
      ) : (
        <DataTable
          nextPageToken={nextPageToken}
          isLoading={isLoading}
          pagerCtx={pagerCtx}
          columns={columns}
          sortModel={sortModel}
          onSortModelChange={setSortModel}
          rows={rows}
        />
      )}
    </>
  );
}
