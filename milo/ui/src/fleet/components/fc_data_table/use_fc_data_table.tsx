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
  type MRT_ColumnDef,
  useMaterialReactTable,
} from 'material-react-table';
import { useEffect, useRef } from 'react';
import { flushSync } from 'react-dom';

export type FC_ColumnDef<
  TData extends MRT_RowData,
  TValue = unknown,
> = MRT_ColumnDef<TData, TValue> & {
  filterRangeMin?: number;
  filterRangeMax?: number;
};

import { MRT_INTERNAL_COLUMNS } from '@/fleet/components/columns/use_mrt_column_management';
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

const SELECT_COL_PADDING = '8px !important';

export const useFCDataTable = <TData extends MRT_RowData>(
  tableOptions: MRT_TableOptions<TData>,
): MRT_TableInstance<TData> => {
  const [settings, setSettings] = useSettings();
  const closeMenuRef = useRef<() => void>(null);

  const defaultOptions: Partial<MRT_TableOptions<TData>> = {
    enableKeyboardShortcuts: false, // Prevents stealing Arrow Keys from Dropdown Menus
    enableColumnFilters: false,
    enableGlobalFilter: false,
    enableHiding: true, // Needed for hide column in menu
    enableColumnResizing: true,
    enableColumnActions: true,
    layoutMode: 'grid',

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
      sx: { border: 'none', boxShadow: 'none' },
    },
    muiTableHeadProps: {
      sx: { boxShadow: 'none' },
    },
    muiTableHeadRowProps: {
      // Force 2-line header height to prevent flex overlap/expansion issues
      sx: { boxShadow: 'none', minHeight: '56px' },
    },

    renderColumnActionsMenuItems: ({ column, closeMenu }) => {
      closeMenuRef.current = closeMenu;
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

      // Material-React-Table's getCanFilter() respects global table states like `enableColumnFilters: false`.
      // We use `enableColumnFilters: false` to hide native inline text inputs, but still want dropdown filtering functionality universally.
      // We evaluate the explicit columnDef configuration, excluding MRT's internal columns.
      const isInternalColumn = MRT_INTERNAL_COLUMNS.has(column.id);

      const isFilterable =
        !isInternalColumn && column.columnDef.enableColumnFilter !== false;

      // Add the filter dropdown item if the column supports filtering
      if (isFilterable) {
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
      const isSelectCol = column.id === 'mrt-row-select';

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
          paddingTop: `var(--cell-padding-vertical) !important`,
          paddingBottom: `var(--cell-padding-vertical) !important`,
          paddingLeft: isSelectCol
            ? SELECT_COL_PADDING
            : `var(--cell-padding-horizontal) !important`,
          paddingRight: isSelectCol
            ? SELECT_COL_PADDING
            : `var(--cell-padding-horizontal) !important`,
        } as SxProps<Theme>,
        'aria-description': title,
        title: title,
      };
    },

    muiColumnActionsButtonProps: {
      className: 'ColumnActionsMenuButton',
      'aria-label': 'Column Actions',
    },

    muiTableBodyCellProps: ({ column }) => {
      const meta = column.columnDef.meta;
      const isHighlighted = meta?.isHighlighted || column.getIsFiltered();
      const isSelectCol = column.id === 'mrt-row-select';

      return {
        className: isHighlighted ? 'column-highlight' : '',
        sx: {
          fontSize: '13px',
          paddingTop: `var(--cell-padding-vertical)`,
          paddingBottom: `var(--cell-padding-vertical)`,
          paddingLeft: isSelectCol
            ? SELECT_COL_PADDING
            : `var(--cell-padding-horizontal)`,
          paddingRight: isSelectCol
            ? SELECT_COL_PADDING
            : `var(--cell-padding-horizontal)`,
          ...(isHighlighted && {
            backgroundColor: `${colors.blue[50]} !important`,
            color: `${colors.blue[600]} !important`,
          }),
        } as SxProps<Theme>,
      };
    },
    muiTableBodyRowProps: {
      sx: {
        '&.MuiTableRow-root.Mui-selected': {
          backgroundColor: `${colors.blue[50]} !important`,
        },
        '&.MuiTableRow-root.Mui-selected:hover': {
          backgroundColor: `${colors.blue[100]} !important`,
        },
        '&.MuiTableRow-root.Mui-selected > .MuiTableCell-root::after': {
          display: 'none', // Hide standard MRT overlay to let our solid MUI color shine
        },
        '&.MuiTableRow-root.Mui-selected > .MuiTableCell-root.column-highlight':
          {
            backgroundColor: `${colors.blue[100]} !important`,
            zIndex: 1,
          },
        '&.MuiTableRow-root.Mui-selected > .MuiTableCell-root': {
          borderBottom: '1px solid divider !important',
        },
      },
    },

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

  const { columns, data, muiTableContainerProps, ...restTableOptions } =
    tableOptions;
  const mergedTableOptions = _.merge(
    {},
    defaultOptions,
    restTableOptions,
  ) as MRT_TableOptions<TData>;

  mergedTableOptions.muiTableContainerProps = (props) => {
    const density = props.table.getState().density;
    const { vertical, horizontal } = getDensityPadding(density);

    let injectedProps: Record<string, unknown> = {};
    if (typeof muiTableContainerProps === 'function') {
      injectedProps =
        (muiTableContainerProps(props) as Record<string, unknown>) || {};
    } else if (muiTableContainerProps) {
      injectedProps = muiTableContainerProps as Record<string, unknown>;
    }

    const defaultSx = {
      maxWidth: '100%',
      overflowX: 'auto',
      '--cell-padding-vertical': `${vertical}px`,
      '--cell-padding-horizontal': `${horizontal}px`,
    };

    let userSxConfig = {};
    if (typeof injectedProps.sx === 'function') {
      // Technically sx can be a function, but that's very hard to merge transparently.
      // We fall back to the user's function if provided, accepting we might lose default padding.
      // For arrays/objects, we eagerly merge.
      userSxConfig = injectedProps.sx;
    } else if (Array.isArray(injectedProps.sx)) {
      userSxConfig = _.merge({}, ...injectedProps.sx);
    } else if (injectedProps.sx) {
      userSxConfig = injectedProps.sx;
    }

    const mergedSx = _.merge({}, defaultSx, userSxConfig);

    return {
      ...injectedProps,
      sx: (typeof injectedProps.sx === 'function'
        ? [defaultSx, injectedProps.sx]
        : mergedSx) as unknown as Record<string, unknown>,
    };
  };

  if (columns) {
    mergedTableOptions.columns = columns;
  }
  if (data) {
    mergedTableOptions.data = data;
  }

  useEffect(() => {
    const handleScroll = () => {
      const closeMenu = closeMenuRef.current;
      if (closeMenu) {
        // We use flushSync to force the menu to close immediately in the same frame.
        // This prevents the menu from briefly flashing/repositioning to the upper left
        // corner due to React's async state batching latency.
        flushSync(() => {
          closeMenu();
        });
      }
    };

    window.addEventListener('scroll', handleScroll, {
      capture: true,
      passive: true,
    });
    return () => {
      window.removeEventListener('scroll', handleScroll, { capture: true });
      closeMenuRef.current = null;
    };
  }, []);

  return useMaterialReactTable(mergedTableOptions);
};
