import ViewColumnOutlined from '@mui/icons-material/ViewColumnOutlined';
import { Box, Button, TablePagination } from '@mui/material';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import {
  MaterialReactTable,
  MRT_ColumnDef,
  MRT_PaginationState,
  MRT_ColumnFiltersState,
  MRT_Updater,
} from 'material-react-table';
import { useMemo, useCallback } from 'react';

import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  nextPageTokenUpdater,
  pageSizeUpdater,
  prevPageTokenUpdater,
  usePagerContext,
} from '@/common/components/params_pager';
import { ColumnsButton } from '@/fleet/components/columns/columns_button';
import {
  getColumnId,
  useMRTColumnManagement,
} from '@/fleet/components/columns/use_mrt_column_management';
import { FCDataTableCopy } from '@/fleet/components/fc_data_table/fc_data_table_copy';
import { FilterOption } from '@/fleet/components/fc_data_table/mrt_filter_menu_item';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { PAGE_TOKEN_PARAM_KEY } from '@/fleet/constants/param_keys';
import {
  useOrderByParam,
  OrderByDirection,
  ORDER_BY_PARAM_KEY,
} from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { InvalidPageTokenAlert } from '@/fleet/utils/invalid-page-token-alert';
import { parseOrderByParam } from '@/fleet/utils/search_param';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  COLUMNS,
  DEFAULT_COLUMNS,
  DEFAULT_SORT_COLUMN_ID,
} from './rri_columns';
import { getRow, type RriGridRow } from './rri_utils';
import { useRriFilters } from './use_rri_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

const getOrderByDto = (sortingArr: { id: string; desc: boolean }[]) => {
  if (sortingArr.length === 0) {
    return `${DEFAULT_SORT_COLUMN_ID} desc`;
  }
  if (sortingArr.length !== 1) {
    return '';
  }
  const sort = sortingArr[0];
  return sort.desc ? `${sort.id} desc` : sort.id;
};

export const ResourceRequestTable = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [, updateOrderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
    pageTokenKey: PAGE_TOKEN_PARAM_KEY,
  });

  const { filterData, setFilters, aipString } = useRriFilters();

  const client = useFleetConsoleClient();

  const { data: filterOptionsData } = useQuery(
    client.GetResourceRequestsMultiselectFilterValues.query({}),
  );

  const columns = useMemo(() => {
    return Object.values(COLUMNS).map((c) => {
      let filterSelectOptions: FilterOption[] | undefined = undefined;

      if (filterOptionsData && 'rriFilterKey' in c && c.rriFilterKey) {
        filterSelectOptions = filterOptionsData[
          c.rriFilterKey as keyof typeof filterOptionsData
        ] as FilterOption[];
      }

      return {
        ...c,
        filterVariant:
          'enableColumnFilter' in c && c.enableColumnFilter === false
            ? undefined
            : 'multi-select',
        filterSelectOptions: (filterSelectOptions ?? []).filter(Boolean),
      } as MRT_ColumnDef<RriGridRow>;
    });
  }, [filterOptionsData]);

  const columnFilters = useMemo<MRT_ColumnFiltersState>(() => {
    return Object.entries(filterData || {})
      .filter(([, val]) => {
        if (Array.isArray(val)) return val.length > 0;
        if (val && typeof val === 'object') return Object.keys(val).length > 0;
        return val !== undefined && val !== null && val !== '';
      })
      .map(([id, value]) => ({
        id,
        value,
      }));
  }, [filterData]);

  const onColumnFiltersChange = useCallback(
    (updater: MRT_Updater<MRT_ColumnFiltersState>) => {
      const newFiltersState =
        typeof updater === 'function' ? updater(columnFilters) : updater;

      const newFilters = Object.fromEntries(
        newFiltersState.map((f: { id: string; value: unknown }) => [
          f.id,
          f.value,
        ]),
      ) as typeof filterData;

      setFilters(newFilters);
    },
    [columnFilters, setFilters],
  );

  const sorting = (searchParams.get(ORDER_BY_PARAM_KEY) ?? '')
    .split(', ')
    .map(parseOrderByParam)
    .filter((orderBy): orderBy is NonNullable<typeof orderBy> => !!orderBy)
    .map((orderBy) => ({
      id: orderBy.field,
      desc: orderBy.direction === OrderByDirection.DESC,
    }));

  const pagination = useMemo<MRT_PaginationState>(
    () => ({
      pageIndex: getCurrentPageIndex(pagerCtx),
      pageSize: getPageSize(pagerCtx, searchParams),
    }),
    [pagerCtx, searchParams],
  );

  const query = useQuery({
    ...client.ListResourceRequests.query({
      filter: aipString,
      orderBy: getOrderByDto(sorting),
      pageSize: getPageSize(pagerCtx, searchParams),
      pageToken: getPageToken(pagerCtx, searchParams),
    }),
    placeholderData: keepPreviousData,
  });

  const countQuery = useQuery({
    ...client.CountResourceRequests.query({
      filter: aipString,
    }),
  });

  const defaultColumnIds = useMemo(
    () =>
      columns
        .filter((c) => DEFAULT_COLUMNS.includes(getColumnId(c)))
        .map((c) => getColumnId(c)),
    [columns],
  );

  const highlightedColumnIds = useMemo(
    () =>
      filterData === undefined
        ? []
        : Object.keys(filterData).filter((key) => {
            const val = filterData[key as keyof typeof filterData];
            if (Array.isArray(val)) return val.length > 0;
            if (val && typeof val === 'object')
              return Object.keys(val).length > 0;
            return val !== undefined && val !== null;
          }),
    [filterData],
  );

  const mrtColumnManager = useMRTColumnManagement<RriGridRow>({
    columns,
    defaultColumnIds,
    localStorageKey: RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY,
    highlightedColumnIds,
  });

  const table = useFCDataTable<RriGridRow>({
    columns: mrtColumnManager.columns,
    enableColumnActions: true,
    manualFiltering: true,
    positionToolbarAlertBanner: 'none',
    enableRowSelection: false,
    renderTopToolbarCustomActions: ({ table }) => (
      <div
        css={{
          width: '100%',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: '8px',
        }}
      >
        <FCDataTableCopy table={table} />
        <ColumnsButton
          allColumns={mrtColumnManager.allColumns}
          visibleColumns={mrtColumnManager.visibleColumnIds}
          onToggleColumn={mrtColumnManager.onToggleColumn}
          resetDefaultColumns={mrtColumnManager.resetDefaultColumns}
          renderTrigger={({ onClick }, ref) => (
            <Button
              ref={ref}
              startIcon={<ViewColumnOutlined sx={{ fontSize: '20px' }} />}
              onClick={onClick}
              color="inherit"
              sx={{
                color: colors.grey[600],
                height: '40px',
                fontSize: '0.875rem',
                textTransform: 'none',
                fontWeight: 500,
              }}
            >
              Columns
            </Button>
          )}
        />
      </div>
    ),
    data: query.data?.resourceRequests.map((rr) => getRow(rr)) ?? [],
    getRowId: (row) => row.id,
    state: {
      sorting,
      columnFilters,
      columnVisibility: mrtColumnManager.columnVisibility,
      showProgressBars: query.isPending || query.isPlaceholderData,
      showAlertBanner: query.isError,
      pagination: pagination,
    },
    onColumnVisibilityChange: mrtColumnManager.setColumnVisibility,
    onColumnFiltersChange,
    muiTopToolbarProps: {
      sx: {
        '& [aria-label="Show/Hide filters"]': {
          display: 'none',
        },
      },
    },

    muiToolbarAlertBannerProps: query.isError
      ? {
          color: 'error',
          children: getErrorMessage(query.error, 'get resource requests'),
        }
      : undefined,
    manualSorting: true,
    onSortingChange: (updater) => {
      const newSorting =
        typeof updater === 'function' ? updater(sorting) : updater;

      updateOrderByParam(
        newSorting
          .map((sort) => {
            if (sort.desc) return `${sort.id} desc`;
            return sort.id;
          })
          .join(', '),
      );

      setSearchParams(
        (prev: URLSearchParams) => emptyPageTokenUpdater(pagerCtx)(prev),
        {
          replace: true,
        },
      );
    },
    enablePagination: false,
    renderBottomToolbarCustomActions: () => (
      <Box sx={{ width: '100%', display: 'flex', justifyContent: 'flex-end' }}>
        <TablePagination
          component="div"
          count={-1}
          page={getCurrentPageIndex(pagerCtx)}
          rowsPerPage={getPageSize(pagerCtx, searchParams)}
          onPageChange={(_, page) => {
            const currentPage = getCurrentPageIndex(pagerCtx);
            const isPrevPage = page < currentPage;
            const isNextPage = page > currentPage;

            setSearchParams((prev: URLSearchParams) => {
              let next = new URLSearchParams(prev);
              if (isPrevPage) {
                next = prevPageTokenUpdater(pagerCtx)(next);
              } else if (isNextPage) {
                next = nextPageTokenUpdater(
                  pagerCtx,
                  query.data?.nextPageToken ?? '',
                )(next);
              }
              return next;
            });
          }}
          onRowsPerPageChange={(e) => {
            setSearchParams(pageSizeUpdater(pagerCtx, Number(e.target.value)));
          }}
          rowsPerPageOptions={DEFAULT_PAGE_SIZE_OPTIONS}
          labelDisplayedRows={({ from, to }) => {
            if (countQuery.data?.total !== undefined) {
              return `${from}-${to} of ${countQuery.data.total}`;
            }
            return `${from}-${to} of ${
              query.data?.nextPageToken ? `more than ${to}` : to
            }`;
          }}
          slotProps={{
            actions: {
              nextButton: {
                disabled: !query.data?.nextPageToken,
              },
            },
            select: {
              MenuProps: {
                sx: { zIndex: 1401 },
                anchorOrigin: { vertical: 'top', horizontal: 'left' },
                transformOrigin: { vertical: 'bottom', horizontal: 'left' },
              },
            },
          }}
        />
      </Box>
    ),
  });

  if (query.isError && query.error?.message.includes('invalid_page_token')) {
    return (
      <InvalidPageTokenAlert
        pagerCtx={pagerCtx}
        setSearchParams={setSearchParams}
      />
    );
  }

  return (
    <div
      css={{
        borderRadius: 4,
        marginTop: 24,
      }}
    >
      <MaterialReactTable table={table} />
    </div>
  );
};
