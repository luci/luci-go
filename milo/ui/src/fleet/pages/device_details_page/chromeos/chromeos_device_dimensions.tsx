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

import ClearIcon from '@mui/icons-material/Clear';
import SearchIcon from '@mui/icons-material/Search';
import {
  Box,
  IconButton,
  InputAdornment,
  TextField,
  Typography,
} from '@mui/material';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_Column,
  MRT_Cell,
} from 'material-react-table';
import { useMemo, useState } from 'react';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { CellWithTooltip } from '@/fleet/components/table/cell_with_tooltip';
import { BotInformation } from '@/fleet/pages/device_details_page/common/bot_information';
import { BotState } from '@/fleet/pages/device_details_page/common/bot_state';
import { colors } from '@/fleet/theme/colors';
import { FC_CellProps } from '@/fleet/types/table';
import { DEVICE_TASKS_SWARMING_HOST } from '@/fleet/utils/builds';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';

import {
  getFieldDefinition,
  ChromeOSDevice,
  KnownChromeOSColumnId,
} from '../../device_list_page/chromeos/chromeos_fields';

const PRIORITY_TRIAGE_LABEL_KEYS = [
  'servo_hostname',
  'label-servo_hostname',
  'servo_port',
  'label-servo_port',
  'servo_serial',
  'label-servo_serial',
  'servo_type',
  'label-servo_type',
  'label-servo_state',
  'label-servo_usb_state',
  'label-pool',
  'label-board',
  'label-model',
  'label-phase',
  'label-rpm_state',
  'ufs_zone',
  'label-zone',
  'location_tag',
  'dut_name',
  'label-associated_hostname',
];

const PRIORITY_INDEX = new Map(
  PRIORITY_TRIAGE_LABEL_KEYS.map((key, idx) => [key, idx]),
);

const CUSTOM_DIMENSION_IDS: KnownChromeOSColumnId[] = [
  'dut_id',
  'realm',
  'state',
  'dut_state',
];

interface ChromeOSDeviceDimensionsProps {
  device?: ChromeOSDevice;
}

export const ChromeOSDeviceDimensions = ({
  device,
}: ChromeOSDeviceDimensionsProps) => {
  const [filterText, setFilterText] = useState('');

  const allRows = useMemo(() => {
    if (!device) return [];

    const custom = CUSTOM_DIMENSION_IDS.map((id) => ({
      id,
      value: String(getFieldDefinition(id).accessorFn(device) ?? ''),
    }));

    if (!device.deviceSpec) return custom;

    const labelRows = Object.keys(device.deviceSpec.labels)
      .filter((key) => !(CUSTOM_DIMENSION_IDS as string[]).includes(key))
      .sort((a, b) => {
        const pa = PRIORITY_INDEX.get(a);
        const pb = PRIORITY_INDEX.get(b);
        if (pa !== undefined && pb !== undefined) return pa - pb;
        if (pa !== undefined) return -1;
        if (pb !== undefined) return 1;
        return a.localeCompare(b);
      })
      .map((label) => ({
        id: label,
        value: device.deviceSpec!.labels[label].values,
      }));

    return [...custom, ...labelRows];
  }, [device]);

  const dimensionRows = useMemo(() => {
    const query = filterText.trim();
    if (!query) return allRows;
    return fuzzySort(query)(allRows, (row) => {
      const valStr = Array.isArray(row.value)
        ? labelValuesToString(row.value)
        : String(row.value ?? '');
      return `${row.id} ${valStr}`;
    })
      .filter((res) => res.score >= 0)
      .map((res) => res.el);
  }, [allRows, filterText]);

  const columns = useMemo<
    MRT_ColumnDef<{ id: string; value: string | readonly string[] }>[]
  >(
    () => [
      {
        accessorKey: 'id',
        header: 'Key',
        size: 200,
      },
      {
        accessorKey: 'value',
        header: 'Value',
        size: 600,
        Cell: ({
          cell,
          row,
          column,
        }: {
          cell: MRT_Cell<
            { id: string; value: string | readonly string[] },
            unknown
          >;
          row: { original: { id: string; value: string | readonly string[] } };
          column: MRT_Column<
            { id: string; value: string | readonly string[] },
            unknown
          >;
        }): React.ReactNode => {
          const fieldKey = row.original.id;
          const value = cell.getValue();

          const def = getFieldDefinition(fieldKey);
          if (def.renderCell && device) {
            return def.renderCell({
              cell: {
                getValue: () => value,
              },
              row: {
                original: device,
              },
              column: {
                ...column,
                id: fieldKey,
              },
            } as unknown as FC_CellProps<ChromeOSDevice>);
          }

          const strValue = Array.isArray(value)
            ? labelValuesToString(value)
            : String(value ?? '');
          return <CellWithTooltip column={column} value={strValue} />;
        },
      },
    ],
    [device],
  );

  const table = useFCDataTable({
    columns,
    data: dimensionRows,
    getRowId: (r) => r.id,
    enablePagination: false,
    enableColumnActions: false,
    enableSorting: false,
    enableColumnFilters: false,
    enableTopToolbar: true,
    enableStickyHeader: true,
    muiTableHeadRowProps: {
      sx: { minHeight: 'unset' },
    },
    muiTableBodyCellProps: {
      sx: {
        wordBreak: 'break-word',
        whiteSpace: 'normal',
      },
    },
    muiTableContainerProps: {
      sx: {
        maxWidth: '100%',
        overflowX: 'hidden',
        maxHeight: '80vh',
        '--cell-padding-horizontal': '16px',
        '& .Mui-TableHeadCell-Content': {
          minHeight: 'unset !important',
        },
      },
    },
  });

  if (device?.deviceSpec === undefined) {
    return <></>;
  }

  return (
    device && (
      <>
        <BotInformation
          swarmingHost={DEVICE_TASKS_SWARMING_HOST}
          dutId={device?.dutId || ''}
        />

        <BotState
          swarmingHost={DEVICE_TASKS_SWARMING_HOST}
          dutId={device?.dutId || ''}
        />

        <Box
          sx={{
            mt: 4,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 2,
            flexWrap: 'wrap',
          }}
        >
          <Typography variant="h5">Device Dimensions</Typography>
          <TextField
            size="small"
            placeholder="Filter dimensions by key or value..."
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
            sx={{
              minWidth: 300,
              '& .MuiOutlinedInput-root': {
                fontSize: 14,
                minHeight: 36,
                '& fieldset': {
                  borderColor: colors.grey[300],
                },
                '&:hover fieldset': {
                  borderColor: colors.grey[500],
                },
                '&:focus-within fieldset': {
                  borderWidth: '1px !important',
                },
              },
            }}
            slotProps={{
              input: {
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchIcon
                      sx={{ fontSize: 18, color: 'text.secondary' }}
                    />
                  </InputAdornment>
                ),
                endAdornment: filterText ? (
                  <InputAdornment position="end">
                    <IconButton
                      size="small"
                      aria-label="Clear filter"
                      onClick={() => setFilterText('')}
                      edge="end"
                    >
                      <ClearIcon sx={{ fontSize: 16 }} />
                    </IconButton>
                  </InputAdornment>
                ) : undefined,
              },
            }}
          />
        </Box>
        <div css={{ marginTop: 16 }}>
          <MaterialReactTable table={table} />
        </div>
      </>
    )
  );
};
