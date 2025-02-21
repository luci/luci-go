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
import { GridColumnVisibilityModel, GridSortModel } from '@mui/x-data-grid';
import { GridApiCommunity } from '@mui/x-data-grid/internals';
import { useQuery } from '@tanstack/react-query';
import _ from 'lodash';
import { useMemo, useState } from 'react';

import {
  getPageSize,
  getPageToken,
  PagerContext,
} from '@/common/components/params_pager';
import { DEFAULT_DEVICE_COLUMNS } from '@/fleet/config/device_config';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  Device,
  ListDevicesRequest,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { DataTable } from '../data_table';
import { getFilterValue } from '../multi_select_filter/search_param_utils/search_param_utils';

import { BASE_DIMENSIONS, getColumns } from './columns';
import { useDevices } from './use_devices';

function getErrorMessage(error: unknown): string {
  if (error instanceof GrpcError) {
    if (error.code === 7) {
      return "You don't have permission to list devices";
    }

    return error.description;
  }
  return 'Unknown error';
}

function getRow(device: Device): Record<string, string> {
  const row: Record<string, string> = Object.fromEntries(
    BASE_DIMENSIONS.map((dim) => [dim.id, dim.getValue(device)]),
  );

  if (device.deviceSpec) {
    for (const label of Object.keys(device.deviceSpec.labels)) {
      // TODO(b/378634266): should be discussed how to show multiple values
      row[label] = device.deviceSpec.labels[label].values
        .concat()
        .sort((a, b) => (a.length < b.length ? 1 : -1))
        .join(', ')
        .toString();
    }
  }

  return row;
}

interface DeviceTableProps {
  gridRef: React.MutableRefObject<GridApiCommunity>;
  pagerCtx: PagerContext;
  totalRowCount: number | undefined;
}

export function DeviceTable({
  gridRef,
  pagerCtx,
  totalRowCount,
}: DeviceTableProps) {
  const [searchParams] = useSyncedSearchParams();
  const [sortModel, setSortModel] = useState<GridSortModel>([]);

  const getOrderByFromSortModel = () => {
    if (sortModel.length !== 1) {
      return '';
    }
    const sortItem = sortModel[0];
    const baseDimension = BASE_DIMENSIONS.filter(
      (dim) => dim.id === sortItem.field,
    )[0];

    const sortKey = baseDimension
      ? baseDimension.id
      : `labels.${sortItem.field}`;

    return sortItem.sort === 'desc' ? `${sortKey} desc` : sortKey;
  };

  const client = useFleetConsoleClient();
  const request = ListDevicesRequest.fromPartial({
    pageSize: getPageSize(pagerCtx, searchParams),
    pageToken: getPageToken(pagerCtx, searchParams),
    orderBy: getOrderByFromSortModel(),
    filter: getFilterValue(searchParams),
  });

  const devicesQuery = useDevices(request);
  const dimensionsQuery = useQuery(client.GetDeviceDimensions.query({}));

  const { devices = [], nextPageToken = '' } = devicesQuery.data || {};

  const columns = useMemo(
    () =>
      dimensionsQuery.data
        ? getColumns(
            // We need to avoid duplicates
            // E.g. `dut_id` is in both base dimensions and labels
            _.uniq(
              Object.keys(dimensionsQuery.data.baseDimensions).concat(
                Object.keys(dimensionsQuery.data.labels),
              ),
            ),
          )
        : [],
    [dimensionsQuery.data],
  );

  return (
    <>
      {devicesQuery.isError || dimensionsQuery.isError ? (
        <Alert severity="error">
          Something went wrong:{' '}
          {getErrorMessage(devicesQuery.error || dimensionsQuery.error)}
        </Alert>
      ) : (
        <DataTable
          gridRef={gridRef}
          nextPageToken={nextPageToken}
          isLoading={devicesQuery.isLoading || dimensionsQuery.isLoading}
          pagerCtx={pagerCtx}
          columns={columns}
          sortModel={sortModel}
          onSortModelChange={setSortModel}
          rows={devices.map(getRow)}
          defaultColumnVisibilityModel={columns.reduce(
            (visibilityModel, column) => ({
              ...visibilityModel,
              [column.field]: DEFAULT_DEVICE_COLUMNS.includes(column.field),
            }),
            {} as GridColumnVisibilityModel,
          )}
          totalRowCount={totalRowCount}
        />
      )}
    </>
  );
}
