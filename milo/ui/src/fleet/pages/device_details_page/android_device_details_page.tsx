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

import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Alert, IconButton, Typography, Button } from '@mui/material';
import { DateTime } from 'luxon';
import { MaterialReactTable, MRT_ColumnDef } from 'material-react-table';
import { useMemo } from 'react';
import { useNavigate, useParams } from 'react-router';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import {
  generateDeviceListURL,
  ANDROID_PLATFORM,
} from '@/fleet/constants/paths';
import { getErrorMessage } from '@/fleet/utils/errors';

import { useAndroidDeviceData } from './use_android_device_data';

export const AndroidDeviceDetailsPage = () => {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const { error, isError, isLoading, device } = useAndroidDeviceData(id);

  const columns = useMemo<
    MRT_ColumnDef<{ key: string; value: React.ReactNode }>[]
  >(
    () => [
      {
        accessorKey: 'key',
        header: 'Label',
        size: 200,
        grow: false, // Prevent growing
      },
      {
        accessorKey: 'value',
        header: 'Value',
        Cell: ({ cell }) => cell.getValue() as React.ReactNode,
      },
    ],
    [],
  );

  const labels = useMemo(() => {
    if (!device) return [];
    const l = Object.entries(device.omnilabSpec?.labels ?? {}).map(
      ([key, value]) => {
        const strVal = labelValuesToString(value.values);
        if (key === 'ufs.last_sync' || key === 'mh.last_sync') {
          const dt = DateTime.fromISO(strVal);
          if (dt.isValid) {
            return {
              key,
              value: <SmartRelativeTimestamp date={dt} />,
            };
          }
        }
        return {
          key,
          value: strVal,
        };
      },
    );
    l.sort((a, b) => a.key.localeCompare(b.key));
    return l;
  }, [device]);

  const table = useFCDataTable({
    columns,
    data: labels,
    enablePagination: false,
    enableTopToolbar: false,
    enableStickyHeader: true,
    muiTableContainerProps: {
      sx: { maxWidth: '100%', overflowX: 'hidden', maxHeight: '80vh' },
    },
  });

  if (isError) {
    return (
      <Alert severity="error">
        Something went wrong: {getErrorMessage(error, 'fetch device')}
      </Alert>
    );
  }

  if (isLoading) {
    return (
      <div css={{ width: '100%', margin: '24px 0px' }}>
        <CentralizedProgress data-testid="loading-spinner" />
      </div>
    );
  }

  if (!device) {
    return <Alert severity="warning">Device {id} not found.</Alert>;
  }

  const hostname = device.omnilabSpec?.labels['hostname']?.values?.[0];
  const hostIp = device.omnilabSpec?.labels['host_ip']?.values?.[0];
  let mhUrl = '';
  if (hostname && hostIp) {
    mhUrl = `https://mobileharness-fe.corp.google.com/devicedetailview/${hostname}/${hostIp}/${device.id}`;
  } else {
    const params = new URLSearchParams();
    params.append('filter', `"id":("${device.id}")`);
    mhUrl = `https://mobileharness-fe.corp.google.com/devicelistview?${params.toString()}`;
  }

  return (
    <div css={{ width: '100%', margin: '24px 0px' }}>
      <div
        css={{
          margin: '0px 24px 24px 24px',
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <IconButton
          onClick={() => navigate(generateDeviceListURL(ANDROID_PLATFORM))}
        >
          <ArrowBackIcon />
        </IconButton>
        <Typography variant="h4">Device details: {id}</Typography>
        <Button
          variant="outlined"
          color="primary"
          startIcon={<OpenInNewIcon />}
          href={mhUrl}
          target="_blank"
          rel="noopener noreferrer"
          style={{ marginLeft: 'auto' }}
        >
          View in Mobile Harness
        </Button>
      </div>

      <div css={{ margin: '0 24px' }}>
        <MaterialReactTable table={table} />
      </div>
    </div>
  );
};
