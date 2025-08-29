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

import { colors } from '@mui/material';
import _ from 'lodash';
import {
  MRT_RowData,
  MRT_TableOptions,
  MRT_TableInstance,
  useMaterialReactTable,
} from 'material-react-table';

import { useSettings } from './use_settings';

export const useFCDataTable = <TData extends MRT_RowData>(
  tableOptions: MRT_TableOptions<TData>,
): MRT_TableInstance<TData> => {
  const [settings, setSettings] = useSettings();

  const defaultOptions: Partial<MRT_TableOptions<TData>> = {
    enableColumnFilters: false,
    enableGlobalFilter: false,
    enableHiding: false,
    enableColumnResizing: true,
    enableColumnActions: false,
    layoutMode: 'grid',
    defaultColumn: {
      grow: true,
      minSize: 10,
    },
    onDensityChange: (updater) => {
      const newDensity =
        typeof updater === 'function'
          ? updater(settings.tableMRT.density)
          : updater;
      setSettings({
        ...settings,
        tableMRT: { ...settings.tableMRT, density: newDensity },
      });
    },
    state: {
      density: settings.tableMRT.density,
    },
    muiPaginationProps: {
      rowsPerPageOptions: [10, 25, 50, 100, 500, 1000],
      showFirstButton: false,
      showLastButton: false,
    },
    muiTablePaperProps: {
      elevation: 0,
      sx: { border: 'none' },
    },
    muiTableHeadCellProps: {
      sx: {
        backgroundColor: colors.grey[100],
        py: 1.6,
        lineHeight: 1.1,
        fontWeight: 500,
        justifyContent: 'center',
        [`& .Mui-TableHeadCell-ResizeHandle-Divider`]: {
          borderWidth: '1px',
        },
        [`& .MuiTableSortLabel-icon`]: {
          opacity: 0,
        },
      },
    },
    muiBottomToolbarProps: {
      sx: {
        boxShadow: 'none',
      },
    },
  };

  const mergedTableOptions = _.merge({}, defaultOptions, tableOptions);

  return useMaterialReactTable(mergedTableOptions);
};
