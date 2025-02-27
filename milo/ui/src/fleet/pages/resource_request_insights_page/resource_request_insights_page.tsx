import styled from '@emotion/styled';
import { Alert, CircularProgress } from '@mui/material';
import { GridColDef, GridSortItem, GridSortModel } from '@mui/x-data-grid';
import { useQuery } from '@tanstack/react-query';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { Pagination } from '@/fleet/components/device_table/pagination';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { toIsoString } from '@/fleet/utils/dates';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ResourceRequest } from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

const Container = styled.div`
  margin: 24px;
`;

interface ColumnDescriptor {
  id: string;
  gridColDef: GridColDef;
  valueGetter: (rr: ResourceRequest) => string;
}

const columns: ColumnDescriptor[] = [
  {
    id: 'rr_id',
    gridColDef: {
      field: 'id',
      headerName: 'RR ID',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => rr.rrId,
  },
  {
    id: 'resource_details',
    gridColDef: {
      field: 'resource_details',
      headerName: 'Resource Details',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => rr.resourceDetails,
  },
  {
    id: 'procurement_end_date',
    gridColDef: {
      field: 'procurement_end_date',
      headerName: 'Procurement End Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.procurementEndDate),
  },
  {
    id: 'build_end_date',
    gridColDef: {
      field: 'build_end_date',
      headerName: 'Build End Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.buildEndDate),
  },
  {
    id: 'qa_end_date',
    gridColDef: {
      field: 'qa_end_date',
      headerName: 'QA End Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.qaEndDate),
  },
  {
    id: 'config_end_date',
    gridColDef: {
      field: 'config_end_date',
      headerName: 'Config End Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.configEndDate),
  },
];

const DEFAULT_SORT_COLUMN: ColumnDescriptor =
  columns.find((c) => c.id === 'rr_id') ?? columns[0];

const getColumnByField = (field: string): ColumnDescriptor | undefined => {
  return columns.find((c) => c.gridColDef.field === field);
};

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
  return `${getColumnByField(sortColumn.field)?.id ?? DEFAULT_SORT_COLUMN.id} ${sortColumn.sort}`;
};

export const ResourceRequestListPage = () => {
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const [orderByParam, updateOrderByParam] = useOrderByParam();
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const sortModel = getSortModelFromOrderByParam(orderByParam);

  const handleSortModelChange = (newSortModel: GridSortModel) => {
    updateOrderByParam(getOrderByParamFromSortModel(newSortModel));
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  const client = useFleetConsoleClient();

  const query = useQuery(
    client.ListResourceRequests.query({
      filter: '', // TODO: b/396079336 add filtering
      orderBy: getOrderByDto(sortModel),
      pageSize: pagerCtx.options.defaultPageSize,
      pageToken: getPageToken(pagerCtx, searchParams),
    }),
  );

  if (query.isError) {
    return <Alert severity="error">Something went wrong</Alert>; // TODO: b/397421370 add nice error handling
  }

  if (query.isLoading || !query.data) {
    return (
      <Container>
        <div css={{ padding: '0 50%' }}>
          <CircularProgress />
        </div>
      </Container>
    );
  }

  const rows: Record<string, string>[] = query.data.resourceRequests.map(
    (resourceRequest) => ({
      id: resourceRequest.rrId,
      resource_details: resourceRequest.resourceDetails,
      procurement_end_date: toIsoString(resourceRequest.procurementEndDate),
      build_end_date: toIsoString(resourceRequest.buildEndDate),
      qa_end_date: toIsoString(resourceRequest.qaEndDate),
      config_end_date: toIsoString(resourceRequest.configEndDate),
    }),
  );

  return (
    <Container>
      {/* TODO: this piece of code is similar to data_table.tsx and could probably be separated to a shared component */}
      <StyledGrid
        columns={columns.map((column) => column.gridColDef)}
        rows={rows}
        slots={{
          pagination: Pagination,
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
          pageSize: pagerCtx.options.defaultPageSize,
        }}
        rowSelection={false}
        sortModel={sortModel}
        sortingMode="server"
        onSortModelChange={handleSortModelChange}
      />
    </Container>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-resource-request-list">
      <FleetHelmet pageTitle="Resource Requests" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-resource-request-list-page"
      >
        <LoggedInBoundary>
          <ResourceRequestListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
