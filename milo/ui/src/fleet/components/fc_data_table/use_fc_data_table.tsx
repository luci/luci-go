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

import {
  ArrowDownward,
  ArrowUpward,
  Clear,
  VisibilityOff,
} from '@mui/icons-material';
import { Divider, ListItemIcon, ListItemText, MenuItem } from '@mui/material';
import _ from 'lodash';
import {
  MRT_RowData,
  MRT_TableOptions,
  MRT_TableInstance,
  type MRT_Column,
  useMaterialReactTable,
} from 'material-react-table';

import {
  highlightedColumnClassName,
  temporaryColumnClassName,
} from '@/fleet/components/columns/use_mrt_column_management';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { FleetColumnHeader } from '@/fleet/components/fc_data_table/fleet_column_header';
import { MRTFilterMenuItem } from '@/fleet/components/fc_data_table/mrt_filter_menu_item';
import {
  useSettings,
  mapMRTToMUI,
  mapMUIToMRT,
} from '@/fleet/hooks/use_settings';
import { colors } from '@/fleet/theme/colors';

const columnHighlightSx = {
  [`&.${highlightedColumnClassName}`]: {
    '--header-bg-color': colors.blue[50],
    backgroundColor: 'var(--header-bg-color)',
    '&:hover': {
      backgroundColor: colors.blue[100],
    },
    '& .Mui-TableHeadCell-Content-Labels': {
      color: colors.blue[600],
    },
  },
  [`&.${temporaryColumnClassName} .Mui-TableHeadCell-Content-Wrapper::after`]: {
    content: '"*"',
    marginLeft: '6px',
    color: colors.blue[600],
  },
};

export const useFCDataTable = <TData extends MRT_RowData>(
  tableOptions: MRT_TableOptions<TData>,
): MRT_TableInstance<TData> => {
  const [settings, setSettings] = useSettings();

  const defaultOptions: Partial<MRT_TableOptions<TData>> = {
    enableKeyboardShortcuts: false, // Prevents stealing Arrow Keys from Dropdown Menus
    enableColumnFilters: false,
    enableGlobalFilter: false,
    enableHiding: true, // Needed for hide column in menu
    enableColumnResizing: true,
    enableColumnActions: true,
    layoutMode: 'grid',
    defaultColumn: {
      grow: 1,
      size: 40,
      Cell: (c) => <EllipsisTooltip>{c.renderedCellValue}</EllipsisTooltip>,
      Header: FleetColumnHeader,
    },
    onDensityChange: (updater) => {
      const newDensity =
        typeof updater === 'function'
          ? updater(mapMUIToMRT(settings.table.density))
          : updater;
      setSettings({
        ...settings,
        table: { ...settings.table, density: mapMRTToMUI(newDensity) },
      });
    },
    state: {
      density: mapMUIToMRT(settings.table.density),
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

    renderColumnActionsMenuItems: ({ column, closeMenu }) => {
      const sortingState = column.getIsSorted();

      const items = [];

      if (column.getCanSort()) {
        if (sortingState !== 'asc') {
          items.push(
            <MenuItem
              key="sort-asc"
              onClick={() => {
                column.toggleSorting(false);
                closeMenu();
              }}
              sx={{ minWidth: 200 }}
            >
              <ListItemIcon>
                <ArrowUpward fontSize="small" />
              </ListItemIcon>
              <ListItemText>Sort by ASC</ListItemText>
            </MenuItem>,
          );
        }

        if (sortingState !== 'desc') {
          items.push(
            <MenuItem
              key="sort-desc"
              onClick={() => {
                column.toggleSorting(true);
                closeMenu();
              }}
            >
              <ListItemIcon>
                <ArrowDownward fontSize="small" />
              </ListItemIcon>
              <ListItemText>Sort by DESC</ListItemText>
            </MenuItem>,
          );
        }

        if (sortingState) {
          items.push(
            <MenuItem
              key="unsort"
              onClick={() => {
                column.clearSorting();
                closeMenu();
              }}
            >
              <ListItemIcon>
                <Clear fontSize="small" />
              </ListItemIcon>
              <ListItemText>Unsort</ListItemText>
            </MenuItem>,
          );
        }
      }

      // Add the filter dropdown item if filter options exist
      if (column.columnDef.filterSelectOptions) {
        if (items.length > 0) {
          items.push(<Divider key="divider-filter" />);
        }
        items.push(
          <MRTFilterMenuItem
            key="filter"
            column={column as unknown as MRT_Column<MRT_RowData, unknown>}
            closeMenu={closeMenu}
          />,
        );
      }

      if (column.getCanHide()) {
        if (items.length > 0) {
          items.push(<Divider key="divider-hide" />);
        }
        items.push(
          <MenuItem
            key="hide"
            onClick={() => {
              column.toggleVisibility(false);
              closeMenu();
            }}
          >
            <ListItemIcon>
              <VisibilityOff fontSize="small" />
            </ListItemIcon>
            <ListItemText>Hide column</ListItemText>
          </MenuItem>,
        );
      }

      return items;
    },

    muiTopToolbarProps: {
      sx: {
        '& [aria-label="Show/Hide columns"]': {
          display: 'none',
        },
      },
    },

    muiTableHeadCellProps: ({ column }) => {
      // Temporary columns don't have visibility state explicitly tracked in MRT's column visibility state.
      // Instead, we use a CSS class to visually indicate they are temporary. Check if it has that class.
      // Actually we don't have direct access to the generated DOM props here.
      // But since this is a callback we can pass down an aria-label or description.
      // Since `columnHighlightSx` handles the UI, we can inject an aria-description if the class applies
      return {
        sx: {
          // Define the default variable for the header cell background color
          '--header-bg-color': colors.grey[100],
          backgroundColor: 'var(--header-bg-color, transparent)',

          fontWeight: 500,
          justifyContent: 'center',
          [`& .Mui-TableHeadCell-ResizeHandle-Divider`]: {
            borderWidth: '1px',
          },
          '& .ColumnActionsMenuButton': {
            opacity: 0,
            transition: 'all 0.2s ease',
            backgroundColor: 'colors.transparent',
            width: '24px',
            height: '24px',
            padding: 0,
            margin: 0,
            flexShrink: 0,
          },
          '&:hover .ColumnActionsMenuButton, &:focus-within .ColumnActionsMenuButton':
            {
              opacity: 1,
            },
          '& .ColumnActionsMenuButton:hover, & .ColumnActionsMenuButton[aria-expanded="true"]':
            {
              backgroundColor: 'rgba(0, 0, 0, 0.04) !important',
              opacity: 1,
            },
          '& .MuiTableSortLabel-root': {
            transition: 'all 0.2s ease',
            opacity: column.getIsSorted() ? 1 : 0,
            padding: '4px',
            borderRadius: '50%',
            flexShrink: 0,
            margin: 0,
            marginLeft: '4px',
            width: '24px',
            height: '24px',
            alignItems: 'center',
            justifyContent: 'center',
          },
          '& .MuiTableSortLabel-icon': {
            margin: 0,
            fontSize: '16px',
            transition: 'all 0.2s ease',
          },
          '&:hover .MuiTableSortLabel-root:not(.Mui-active), &:focus-within .MuiTableSortLabel-root:not(.Mui-active)':
            {
              opacity: 1,
              backgroundColor: 'var(--header-bg-color, transparent)',
            },
          '& .MuiTableSortLabel-root:hover': {
            backgroundColor: 'rgba(0, 0, 0, 0.04) !important',
          },
          [`& .Mui-TableHeadCell-Content-Wrapper`]: {
            whiteSpace: 'normal',
            width: '100%',
            display: '-webkit-box',
            WebkitBoxOrient: 'vertical',
            WebkitLineClamp: 2,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
          },
          [`& .Mui-TableHeadCell-Content-Labels`]: {
            overflow: 'hidden',
            flex: 1,
          },
          [`& .MuiTypography-root`]: {
            lineHeight: 'normal',
          },
          ...columnHighlightSx,
        },
        // Use the meta property to determine if the column is temporary
        'aria-description': (column.columnDef.meta as { isTemporary?: boolean })
          ?.isTemporary
          ? 'Column visible because of active filter'
          : undefined,
        title: (column.columnDef.meta as { isTemporary?: boolean })?.isTemporary
          ? 'Column visible because of active filter'
          : undefined,
      };
    },
    muiColumnActionsButtonProps: {
      className: 'ColumnActionsMenuButton',
      'aria-label': 'Column Actions',
    },
    muiTableBodyCellProps: {
      sx: {
        ...columnHighlightSx,
      },
    },
    muiTableContainerProps: { sx: { maxWidth: '100%', overflowX: 'auto' } },
    muiBottomToolbarProps: {
      sx: {
        boxShadow: 'none',
      },
    },
  };

  const mergedTableOptions = _.merge({}, defaultOptions, tableOptions);

  return useMaterialReactTable(mergedTableOptions);
};
