import { Alert, CircularProgress, Container } from '@mui/material';
import {
  GridColumnVisibilityModel,
  GridSortItem,
  GridSortModel,
} from '@mui/x-data-grid';
import { useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';

import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { ColumnMenu } from '@/fleet/components/device_table/column_menu';
import { Pagination } from '@/fleet/components/device_table/pagination';
import { useParamsAndLocalStorage } from '@/fleet/components/device_table/use_params_and_local_storage';
import { RriTableToolbar } from '@/fleet/components/resource_request_insights/rri_table_toolbar';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { getVisibilityModel } from '@/fleet/utils/search_param';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  DEFAULT_SORT_COLUMN,
  getColumnByField,
  RRI_COLUMNS,
  RriColumnDescriptor,
  RriGridRow,
} from './rri_columns';
import { useRriFilters } from './use_rri_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

const getOrderByParamFromSortModel = (sortModel: GridSortModel) => {
  if (sortModel.length !== 1) {
    return '';
  }

  const sortColumn = sortModel[0];
  if (sortColumn.sort === 'asc') {
    return sortColumn.field;
  }
  return `${sortColumn.field} ${sortColumn.sort}`;
};

const getSortModelFromOrderByParam = (orderByParam: string): GridSortItem[] => {
  if (orderByParam === '') {
    return [];
  }
  const [field, sort] = orderByParam.split(' ');
  let actualSort: 'asc' | 'desc' = 'asc';
  if (sort === 'desc') {
    actualSort = 'desc';
  }
  return [
    {
      field: field,
      sort: actualSort,
    },
  ];
};

const getOrderByDto = (sortModel: GridSortModel) => {
  if (sortModel.length !== 1) {
    return `${DEFAULT_SORT_COLUMN.id}`;
  }
  const sortColumn = sortModel[0];

  const sortColumnKey =
    getColumnByField(sortColumn.field)?.id ?? DEFAULT_SORT_COLUMN.id;

  if (sortColumn.sort === 'asc') {
    return sortColumnKey;
  }

  return `${sortColumnKey} desc`;
};

export const ResourceRequestTable = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam, updateOrderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const { aipString } = useRriFilters();

  const sortModel = getSortModelFromOrderByParam(orderByParam);

  const client = useFleetConsoleClient();

  const query = useQuery(
    client.ListResourceRequests.query({
      filter: aipString,
      orderBy: getOrderByDto(sortModel),
      pageSize: getPageSize(pagerCtx, searchParams),
      pageToken: getPageToken(pagerCtx, searchParams),
    }),
  );

  const [visibleColumns, setVisibleColumns] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY,
    RRI_COLUMNS.filter((column: RriColumnDescriptor) => column.isDefault).map(
      (column: RriColumnDescriptor) => column.gridColDef.field,
    ),
  );

  const onColumnVisibilityModelChange = (
    newColumnVisibilityModel: GridColumnVisibilityModel,
  ) => {
    setVisibleColumns(
      Object.entries(newColumnVisibilityModel)
        .filter(([_key, val]) => val)
        .map(([key, _val]) => key),
    );
  };

  const columns = useMemo(() => {
    return RRI_COLUMNS.map((column) => column.gridColDef);
  }, []);

  const handleSortModelChange = (newSortModel: GridSortModel) => {
    updateOrderByParam(getOrderByParamFromSortModel(newSortModel));
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  if (query.isError) {
    return <Alert severity="error">Something went wrong</Alert>; // TODO: b/397421370 add nice error handling
  }

  if (query.isPending || !query.data) {
    return (
      <Container>
        <div css={{ padding: '0 50%' }}>
          <CircularProgress data-testid="loading-spinner" />
        </div>
      </Container>
    );
  }

  const rows: RriGridRow[] = query.data.resourceRequests.map(
    (resourceRequest, index) => {
      const row = { id: index.toString() } as RriGridRow;
      for (const column of RRI_COLUMNS) {
        column.assignValue(resourceRequest, row);
      }
      return row;
    },
  );

  return (
    <div
      css={{
        borderRadius: 4,
        marginTop: 24,
      }}
    >
      {/* TODO: this piece of code is similar to data_table.tsx and could probably be separated to a shared component */}
      <StyledGrid
        columns={columns}
        rows={rows}
        slots={{
          pagination: Pagination,
          columnMenu: ColumnMenu,
          toolbar: RriTableToolbar,
        }}
        slotProps={{
          pagination: {
            pagerCtx: pagerCtx,
            nextPageToken: query.data.nextPageToken,
          },
        }}
        paginationMode="server"
        pageSizeOptions={pagerCtx.options.pageSizeOptions}
        rowCount={-1}
        paginationModel={{
          page: getCurrentPageIndex(pagerCtx),
          pageSize: getPageSize(pagerCtx, searchParams),
        }}
        rowSelection={false}
        sortModel={sortModel}
        sortingMode="server"
        onSortModelChange={handleSortModelChange}
        columnVisibilityModel={getVisibilityModel(
          RRI_COLUMNS.map(
            (column: RriColumnDescriptor) => column.gridColDef.field,
          ),
          visibleColumns,
        )}
        onColumnVisibilityModelChange={onColumnVisibilityModelChange}
      />
    </div>
  );
};
