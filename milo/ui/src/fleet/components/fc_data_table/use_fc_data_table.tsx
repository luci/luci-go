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
  type MRT_ColumnDef,
  useMaterialReactTable,
} from 'material-react-table';

import { MRT_INTERNAL_COLUMNS } from '@/fleet/components/columns/use_mrt_column_management';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { FCFilterMenuItem } from '@/fleet/components/fc_data_table/fc_filter_menu_item';
import { FleetColumnHeader } from '@/fleet/components/fc_data_table/fleet_column_header';
import {
  fleetTableBodyCellFlatSx,
  fleetTableBodyCellSx,
  fleetTableHeaderSx,
  getDensityPadding,
  TABLE_HEADER_MIN_HEIGHT,
} from '@/fleet/components/fc_data_table/fleet_column_header.styles';
import { MRTFilterMenuItem_OLD } from '@/fleet/components/fc_data_table/mrt_filter_menu_item';
import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { FilterCategory } from '@/fleet/components/filters/use_filters';
import {
  mapMRTToMUI,
  mapMUIToMRT,
  useSettings,
} from '@/fleet/hooks/use_settings';
import { iconButtonSx, surfaceCardClippedSx } from '@/fleet/theme/recipes';
import { borders, controlHeights, fleetColors } from '@/fleet/theme/tokens';

import { MenuScrollCloser } from './menu_scroll_closer';

export type FC_ColumnDef<
  TData extends MRT_RowData,
  TValue = unknown,
> = MRT_ColumnDef<TData, TValue> & {
  filterRangeMin?: number;
  filterRangeMax?: number;
  filterRangeUnit?: string;
  filterKey?: string;
};

// This type exists to be able to add custom fields to MRT_TableOptions (such as filterValue)
// and becuase the library provided type doesn't allow to specify the column value
//
// For everything to keep working this needs to be kept as a superset of the original MRT_TableOptions,
// please be careful when modifing existing fields
export type FC_TableOptions<TData extends MRT_RowData, TValue = unknown> = Omit<
  MRT_TableOptions<TData>,
  'columns' | 'data'
> & {
  data: readonly TData[];
  columns: FC_ColumnDef<TData, TValue>[];
  filterValues?: Record<string, FilterCategory>;
};

const DEFAULT_DISPLAY_COLUMN_DEF_OPTIONS = {
  'mrt-row-select': {
    size: 40,
  },
};

const DEFAULT_LOCALIZATION = {
  sortByColumnAsc: 'Sort by {column} ascending (Shift + Click to multi-sort)',
  sortByColumnDesc: 'Sort by {column} descending (Shift + Click to multi-sort)',
  sortedByColumnAsc:
    'Sorted by {column} ascending (Shift + Click to multi-sort)',
  sortedByColumnDesc:
    'Sorted by {column} descending (Shift + Click to multi-sort)',
};

const DEFAULT_MUI_PAGINATION_PROPS = {
  rowsPerPageOptions: [10, 25, 50, 100, 500, 1000],
  showFirstButton: false,
  showLastButton: false,
  SelectProps: {
    MenuProps: {
      anchorOrigin: { vertical: 'top' as const, horizontal: 'left' as const },
      transformOrigin: {
        vertical: 'bottom' as const,
        horizontal: 'left' as const,
      },
    },
  },
};

const DEFAULT_MRT_THEME = {
  baseBackgroundColor: fleetColors.surface,
};

const DEFAULT_MUI_TABLE_PAPER_PROPS = {
  variant: 'surface' as const,
  elevation: 0,
  sx: surfaceCardClippedSx,
};

const DEFAULT_MUI_TABLE_HEAD_PROPS = {
  sx: { boxShadow: 'none' },
};

const DEFAULT_MUI_TABLE_HEAD_ROW_PROPS = {
  // Force 2-line header height to prevent flex overlap/expansion issues
  sx: { boxShadow: 'none', minHeight: TABLE_HEADER_MIN_HEIGHT },
};

const DEFAULT_MUI_TOP_TOOLBAR_PROPS = {
  sx: {
    backgroundColor: fleetColors.surface,
    borderBottom: borders.divider,
    minHeight: TABLE_HEADER_MIN_HEIGHT,
    py: 1,
    px: 1.75,
    display: 'flex',
    alignItems: 'center',
    '& > .MuiBox-root': {
      alignItems: 'center',
      minHeight: controlHeights.button,
    },
    '& .MuiIconButton-root': {
      ...iconButtonSx,
      p: 0.75,
    },
    '& [aria-label="Show/Hide columns"]': {
      display: 'none',
    },
  },
};

const DEFAULT_MUI_COLUMN_ACTIONS_BUTTON_PROPS = {
  className: 'ColumnActionsMenuButton',
  'aria-label': 'Column Actions',
};

const DEFAULT_MUI_TABLE_BODY_ROW_PROPS = {
  sx: {
    '& > .MuiTableCell-root': {
      borderBottom: borders.divider,
    },
    '&:not(.Mui-selected):hover > .MuiTableCell-root': {
      backgroundColor: fleetColors.surfaceTonal,
    },
  },
};

const DEFAULT_MUI_TABLE_PROPS = {
  sx: {
    tableLayout: 'fixed',
  },
};

const DEFAULT_MUI_BOTTOM_TOOLBAR_PROPS = {
  sx: {
    backgroundColor: fleetColors.canvas,
    borderTop: borders.default,
    boxShadow: 'none',
  },
};

const DEFAULT_CONTAINER_SX_BY_DENSITY = (
  ['spacious', 'comfortable', 'compact'] as const
).reduce(
  (acc, d) => {
    const { vertical, horizontal } = getDensityPadding(d);
    acc[d] = {
      maxWidth: '100%',
      overflowX: 'auto',
      overflowY: 'auto',
      maxHeight: 'calc(100vh - 200px)',
      '--cell-padding-vertical': `${vertical}px`,
      '--cell-padding-horizontal': `${horizontal}px`,
    };
    return acc;
  },
  {} as Record<string, Record<string, unknown>>,
);

function mergeDefaultProp<TDefault, TOverride>(
  defaultVal: TDefault,
  overrideVal: TOverride,
): TDefault | NonNullable<TOverride> {
  if (overrideVal === undefined) {
    return defaultVal;
  }
  if (typeof overrideVal === 'function') {
    if (defaultVal && typeof defaultVal === 'object') {
      return ((...args: unknown[]) =>
        _.merge(
          {},
          defaultVal,
          (overrideVal as (...a: unknown[]) => unknown)(...args),
        )) as unknown as NonNullable<TOverride>;
    }
    return overrideVal as NonNullable<TOverride>;
  }
  return _.merge({}, defaultVal, overrideVal);
}

export const useFCDataTable = <TData extends MRT_RowData, TValue = unknown>(
  tableOptions: FC_TableOptions<TData, TValue> & {
    filterValues?: Record<string, FilterCategory>;
    error?: string;
  },
): MRT_TableInstance<TData> => {
  const [settings, setSettings] = useSettings();

  const defaultOptions: Partial<FC_TableOptions<TData, TValue>> = {
    enableKeyboardShortcuts: false, // Prevents stealing Arrow Keys from Dropdown Menus
    enableColumnFilters: false,
    enableGlobalFilter: false,
    enableHiding: true, // Needed for hide column in menu
    enableColumnResizing: true,
    enableColumnActions: true,
    layoutMode: 'grid',
    manualSorting: true,
    enableRowVirtualization: process.env.NODE_ENV !== 'test',

    displayColumnDefOptions: DEFAULT_DISPLAY_COLUMN_DEF_OPTIONS,
    defaultColumn: {
      grow: 1,
      size: 40,
      Cell: (c) => <EllipsisTooltip>{c.renderedCellValue}</EllipsisTooltip>,
      Header: FleetColumnHeader,
    },
    // Override MRT default sort tooltips to include info about multi-sort.
    localization: DEFAULT_LOCALIZATION,
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

    muiPaginationProps: DEFAULT_MUI_PAGINATION_PROPS,
    mrtTheme: DEFAULT_MRT_THEME,
    muiTablePaperProps: DEFAULT_MUI_TABLE_PAPER_PROPS,
    muiTableHeadProps: DEFAULT_MUI_TABLE_HEAD_PROPS,
    muiTableHeadRowProps: DEFAULT_MUI_TABLE_HEAD_ROW_PROPS,

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

      // Material-React-Table's getCanFilter() respects global table states like `enableColumnFilters: false`.
      // We use `enableColumnFilters: false` to hide native inline text inputs, but still want dropdown filtering functionality universally.
      // We evaluate the explicit columnDef configuration, excluding MRT's internal columns.
      const isInternalColumn = MRT_INTERNAL_COLUMNS.has(column.id);

      const isFilterable =
        !isInternalColumn && column.columnDef.enableColumnFilter !== false;

      // Add the filter dropdown item if the column supports filtering
      if (isFilterable) {
        const colDef = column.columnDef as FC_ColumnDef<TData>;
        const filterKey = colDef.filterKey || column.id;
        const filter = filterValues?.[filterKey];
        let filterItem = null;

        if (filter) {
          filterItem = (
            <FCFilterMenuItem
              key="filter"
              filter={filter}
              closeMenu={closeMenu}
              // TODO: infering if searchinput should be here is not ideal. Either the filter value should hold this as a flag or
              // it should be rendered by the StringListFilterCategory itself
              enableSearchInput={filter instanceof StringListFilterCategory}
            />
          );
        } else if (column.columnDef.filterSelectOptions || !filterValues) {
          filterItem = (
            <MRTFilterMenuItem_OLD
              key="filter"
              column={column as unknown as MRT_Column<MRT_RowData, unknown>}
              closeMenu={closeMenu}
            />
          );
        }

        if (filterItem) {
          if (items.length > 0) {
            items.push(<Divider key="divider-filter" />);
          }
          items.push(filterItem);
        }
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

      items.push(
        <MenuScrollCloser key="scroll-closer" closeMenu={closeMenu} />,
      );

      return items;
    },

    muiTopToolbarProps: DEFAULT_MUI_TOP_TOOLBAR_PROPS,

    muiTableHeadCellProps: ({ column }) => {
      const isSelectCol = column.id === 'mrt-row-select';

      const meta = column.columnDef.meta;
      const isTemporary = meta?.isTemporary;
      const isHighlighted = meta?.isHighlighted || column.getIsFiltered();
      const title = isTemporary
        ? 'Column visible because of active filter'
        : undefined;
      const classNames = [
        isHighlighted ? 'column-highlight' : '',
        isSelectCol ? 'select-col' : '',
      ]
        .filter(Boolean)
        .join(' ');

      return {
        className: classNames,
        sx: fleetTableHeaderSx,
        'aria-description': title,
        title: title,
      };
    },

    muiColumnActionsButtonProps: DEFAULT_MUI_COLUMN_ACTIONS_BUTTON_PROPS,

    muiTableBodyCellProps: ({ column }) => {
      const meta = column.columnDef.meta;
      const isHighlighted = meta?.isHighlighted || column.getIsFiltered();
      const isSelectCol = column.id === 'mrt-row-select';
      const classNames = [
        isHighlighted ? 'column-highlight' : '',
        isSelectCol ? 'select-col' : '',
      ]
        .filter(Boolean)
        .join(' ');

      return {
        className: classNames,
        sx: classNames ? fleetTableBodyCellSx : fleetTableBodyCellFlatSx,
      };
    },
    muiTableBodyRowProps: DEFAULT_MUI_TABLE_BODY_ROW_PROPS,

    muiTableProps: DEFAULT_MUI_TABLE_PROPS,
    muiBottomToolbarProps: DEFAULT_MUI_BOTTOM_TOOLBAR_PROPS,
  };

  const {
    columns,
    data,
    muiTableContainerProps,
    filterValues,
    error,
    meta,
    ...restTableOptions
  } = tableOptions;

  const errorOptions: Partial<MRT_TableOptions<TData>> = error
    ? {
        state: {
          showAlertBanner: true,
        },
        muiToolbarAlertBannerProps: {
          color: 'error',
          children: error,
        },
      }
    : {};

  const mergedTableOptions = {
    ...defaultOptions,
    ...errorOptions,
    ...restTableOptions,
    state: {
      ...defaultOptions.state,
      ...errorOptions.state,
      ...restTableOptions.state,
    },
    muiToolbarAlertBannerProps: restTableOptions.muiToolbarAlertBannerProps
      ? mergeDefaultProp(
          errorOptions.muiToolbarAlertBannerProps,
          restTableOptions.muiToolbarAlertBannerProps,
        )
      : errorOptions.muiToolbarAlertBannerProps,
    displayColumnDefOptions: mergeDefaultProp(
      DEFAULT_DISPLAY_COLUMN_DEF_OPTIONS,
      restTableOptions.displayColumnDefOptions,
    ),
    defaultColumn: mergeDefaultProp(
      defaultOptions.defaultColumn,
      restTableOptions.defaultColumn,
    ),
    localization: mergeDefaultProp(
      DEFAULT_LOCALIZATION,
      restTableOptions.localization,
    ),
    muiPaginationProps: mergeDefaultProp(
      DEFAULT_MUI_PAGINATION_PROPS,
      restTableOptions.muiPaginationProps,
    ),
    mrtTheme: mergeDefaultProp(DEFAULT_MRT_THEME, restTableOptions.mrtTheme),
    muiTablePaperProps: mergeDefaultProp(
      DEFAULT_MUI_TABLE_PAPER_PROPS,
      restTableOptions.muiTablePaperProps,
    ),
    muiTableHeadProps: mergeDefaultProp(
      DEFAULT_MUI_TABLE_HEAD_PROPS,
      restTableOptions.muiTableHeadProps,
    ),
    muiTableHeadRowProps: mergeDefaultProp(
      DEFAULT_MUI_TABLE_HEAD_ROW_PROPS,
      restTableOptions.muiTableHeadRowProps,
    ),
    muiTopToolbarProps: mergeDefaultProp(
      DEFAULT_MUI_TOP_TOOLBAR_PROPS,
      restTableOptions.muiTopToolbarProps,
    ),
    muiColumnActionsButtonProps: mergeDefaultProp(
      DEFAULT_MUI_COLUMN_ACTIONS_BUTTON_PROPS,
      restTableOptions.muiColumnActionsButtonProps,
    ),
    muiTableBodyRowProps: mergeDefaultProp(
      DEFAULT_MUI_TABLE_BODY_ROW_PROPS,
      restTableOptions.muiTableBodyRowProps,
    ),
    muiTableProps: mergeDefaultProp(
      DEFAULT_MUI_TABLE_PROPS,
      restTableOptions.muiTableProps,
    ),
    muiBottomToolbarProps: mergeDefaultProp(
      DEFAULT_MUI_BOTTOM_TOOLBAR_PROPS,
      restTableOptions.muiBottomToolbarProps,
    ),
  } as FC_TableOptions<TData, TValue>;

  if (meta) {
    mergedTableOptions.meta = meta;
  }

  mergedTableOptions.muiTableContainerProps = (props) => {
    const density = props.table.getState().density;

    let injectedProps: Record<string, unknown> = {};
    if (typeof muiTableContainerProps === 'function') {
      injectedProps =
        (muiTableContainerProps(props) as Record<string, unknown>) || {};
    } else if (muiTableContainerProps) {
      injectedProps = muiTableContainerProps as Record<string, unknown>;
    }

    const defaultSx =
      DEFAULT_CONTAINER_SX_BY_DENSITY[density] ??
      (() => {
        const { vertical, horizontal } = getDensityPadding(density);
        return {
          maxWidth: '100%',
          overflowX: 'auto',
          overflowY: 'auto',
          maxHeight: 'calc(100vh - 200px)',
          '--cell-padding-vertical': `${vertical}px`,
          '--cell-padding-horizontal': `${horizontal}px`,
        };
      })();

    if (!injectedProps.sx) {
      return {
        ...injectedProps,
        sx: defaultSx,
      };
    }

    let userSxConfig = {};
    if (typeof injectedProps.sx === 'function') {
      // Technically sx can be a function, but that's very hard to merge transparently.
      //
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

  /// SAFETY: This type casting is ok because FC_TableOptions is constrained to be a super set of MRT_TableOptions
  /// @type {FC_TableOptions}
  return useMaterialReactTable(
    mergedTableOptions as unknown as MRT_TableOptions<TData>,
  );
};
