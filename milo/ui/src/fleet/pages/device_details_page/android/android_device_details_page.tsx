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
import {
  Alert,
  IconButton,
  Typography,
  Button,
  TextField,
} from '@mui/material';
import { DateTime } from 'luxon';
import { MaterialReactTable, MRT_ColumnDef } from 'material-react-table';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import AlertWithFeedback from '@/fleet/components/feedback/alert_with_feedback';
import { SmartRelativeTimestamp } from '@/fleet/components/smart_relative_timestamp';
import {
  generateDeviceListURL,
  ANDROID_PLATFORM,
  generateAndroidDeviceDetailsURL,
} from '@/fleet/constants/paths';
import { getErrorMessage } from '@/fleet/utils/errors';

import { useAndroidDeviceData } from './use_android_device_data';

const useNavigatedFromLink = () => {
  const { state } = useLocation();

  const [navigatedFromLink, setNavigatedFromLink] = useState<
    string | undefined
  >(undefined);

  // state from useLocation is lost on rerenders so we need to save it in a react state
  useEffect(() => {
    if (state) {
      setNavigatedFromLink(state.navigatedFromLink);
    }
  }, [state]);

  return navigatedFromLink;
};

export const AndroidDeviceDetailsPage = () => {
  const { id = '' } = useParams();
  const [deviceIdInputValue, setDeviceIdInputValue] = useState(id);
  const navigate = useNavigate();
  const { error, isError, isLoading, device } = useAndroidDeviceData(id);

  const navigatedFromLink = useNavigatedFromLink();
  const deviceIdInputRef = useRef<HTMLInputElement>(null);

  const navigateToDeviceIfChanged = (deviceId: string) => {
    const parts = location.pathname.toString().split('/');
    const urlId = parts[parts.length - 1];
    if (urlId !== deviceId) {
      navigate(`${generateAndroidDeviceDetailsURL(deviceId)}`);
    }
  };

  useEffect(() => {
    const f = (e: KeyboardEvent) => {
      if (e.key === '/') {
        if (!deviceIdInputRef.current?.contains(document.activeElement)) {
          e.preventDefault();
          deviceIdInputRef.current?.focus();
        }
      }
    };

    window.addEventListener('keydown', f);
    return () => {
      window.removeEventListener('keydown', f);
    };
  }, []);

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
        <Typography variant="h4" sx={{ whiteSpace: 'nowrap' }}>
          Device details:
        </Typography>
        <TextField
          inputRef={deviceIdInputRef}
          variant="standard"
          value={deviceIdInputValue}
          onChange={(event) => setDeviceIdInputValue(event.target.value)}
          slotProps={{ htmlInput: { sx: { fontSize: 24 } } }}
          fullWidth
          onBlur={(e) => {
            navigateToDeviceIfChanged(e.target.value);
          }}
          onKeyDown={(e) => {
            const target = e.target as HTMLInputElement;
            if (e.key === 'Enter') {
              navigateToDeviceIfChanged(target.value);
            }
          }}
        />
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

  const hostname = device?.omnilabSpec?.labels['hostname']?.values?.[0];
  const hostIp = device?.omnilabSpec?.labels['host_ip']?.values?.[0];
  let mhUrl = '';
  if (hostname && hostIp) {
    mhUrl = `https://mobileharness-fe.corp.google.com/devicedetailview/${hostname}/${hostIp}/${device.id}`;
  } else {
    const params = new URLSearchParams();
    params.append('filter', `"id":("${device?.id}")`);
    mhUrl = `https://mobileharness-fe.corp.google.com/devicelistview?${params.toString()}`;
  }

  const headerSection = (
    <div
      css={{
        margin: '24px 24px 8px 24px',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
      }}
    >
      <IconButton
        onClick={() => {
          if (navigatedFromLink) {
            navigate(navigatedFromLink);
          } else {
            navigate(generateDeviceListURL(ANDROID_PLATFORM));
          }
        }}
      >
        <ArrowBackIcon />
      </IconButton>
      <Typography variant="h4" sx={{ whiteSpace: 'nowrap' }}>
        Device details:
      </Typography>
      <TextField
        inputRef={deviceIdInputRef}
        variant="standard"
        value={deviceIdInputValue}
        onChange={(event) => setDeviceIdInputValue(event.target.value)}
        slotProps={{ htmlInput: { sx: { fontSize: 24 } } }}
        fullWidth
        onBlur={(e) => {
          navigateToDeviceIfChanged(e.target.value);
        }}
        onKeyDown={(e) => {
          const target = e.target as HTMLInputElement;
          if (e.key === 'Enter') {
            navigateToDeviceIfChanged(target.value);
          }
        }}
      />
      {device && (
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
      )}
    </div>
  );

  return (
    <div css={{ width: '100%', margin: '24px 0px' }}>
      {headerSection}

      {isError && (
        <Alert severity="error" sx={{ margin: '0 24px' }}>
          Something went wrong: {getErrorMessage(error, 'fetch device')}
        </Alert>
      )}

      {!isError && !device && (
        <AlertWithFeedback
          title="Device not found!"
          bugErrorMessage={`Device not found: ${id}`}
        >
          <p>
            Oh no! The device <code>{id}</code> you are looking for was not
            found.
          </p>
        </AlertWithFeedback>
      )}

      {device && (
        <div css={{ margin: '0 24px' }}>
          {<MaterialReactTable table={table} />}
        </div>
      )}
    </div>
  );
};
