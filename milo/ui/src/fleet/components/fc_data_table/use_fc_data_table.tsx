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
import {
  Divider,
  ListItemIcon,
  ListItemText,
  MenuItem,
  SxProps,
  Theme,
} from '@mui/material';
import _ from 'lodash';
import {
  MRT_RowData,
  MRT_TableOptions,
  MRT_TableInstance,
  type MRT_Column,
  useMaterialReactTable,
} from 'material-react-table';

import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { FleetColumnHeader } from '@/fleet/components/fc_data_table/fleet_column_header';
import {
  fleetTableHeaderSx,
  getDensityPadding,
} from '@/fleet/components/fc_data_table/fleet_column_header.styles';
import { MRTFilterMenuItem } from '@/fleet/components/fc_data_table/mrt_filter_menu_item';
import {
  mapMRTToMUI,
  mapMUIToMRT,
  useSettings,
} from '@/fleet/hooks/use_settings';
import { colors } from '@/fleet/theme/colors';

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
    layoutMode: 'semantic', // Avoid grid mode as it causes massive DOM tree lag on column resize
    displayColumnDefOptions: {
      'mrt-row-select': {
        size: 40,
      },
    },
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
      SelectProps: {
        MenuProps: {
          anchorOrigin: { vertical: 'top', horizontal: 'left' },
          transformOrigin: { vertical: 'bottom', horizontal: 'left' },
        },
      },
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

    muiTableHeadCellProps: ({ table, column }) => {
      const density = table.getState().density;
      const { vertical, horizontal } = getDensityPadding(density);

      const meta = column.columnDef.meta;
      const isTemporary = meta?.isTemporary;
      const isHighlighted = meta?.isHighlighted || column.getIsFiltered();
      const title = isTemporary
        ? 'Column visible because of active filter'
        : undefined;

      return {
        className: isHighlighted ? 'column-highlight' : '',
        sx: {
          ...(fleetTableHeaderSx as Record<string, unknown>),
          paddingTop: `${vertical}px !important`,
          paddingBottom: `${vertical}px !important`,
          paddingLeft: `${horizontal}px !important`,
          paddingRight: `${horizontal}px !important`,
        } as SxProps<Theme>,
        'aria-description': title,
        title: title,
      };
    },

    muiColumnActionsButtonProps: {
      className: 'ColumnActionsMenuButton',
      'aria-label': 'Column Actions',
    },

    muiTableBodyCellProps: ({ table, column }) => {
      const density = table.getState().density;
      const meta = column.columnDef.meta;
      const isHighlighted = meta?.isHighlighted || column.getIsFiltered();

      const { vertical, horizontal } = getDensityPadding(density);

      return {
        className: isHighlighted ? 'column-highlight' : '',
        sx: {
          paddingTop: `${vertical}px !important`,
          paddingBottom: `${vertical}px !important`,
          paddingLeft: `${horizontal}px !important`,
          paddingRight: `${horizontal}px !important`,
          backgroundColor: isHighlighted ? `${colors.blue[50]}` : undefined,
          color: isHighlighted ? `${colors.blue[600]}` : undefined,
        } as SxProps<Theme>,
      };
    },
    muiTableContainerProps: { sx: { maxWidth: '100%', overflowX: 'auto' } },
    muiTableProps: {
      sx: {
        tableLayout: 'fixed',
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
