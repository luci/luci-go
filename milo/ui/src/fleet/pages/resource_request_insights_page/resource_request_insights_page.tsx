import styled from '@emotion/styled';
import { Alert, CircularProgress } from '@mui/material';
import { GridColDef, GridSortModel } from '@mui/x-data-grid';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  getCurrentPageIndex,
  usePagerContext,
} from '@/common/components/params_pager';
import { Pagination } from '@/fleet/components/data_table/pagination';
import { StyledGrid } from '@/fleet/components/data_table/styled_data_grid';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { toIsoString } from '@/fleet/utils/dates';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

const Container = styled.div`
  margin: 24px;
`;

export const ResourceRequestListPage = () => {
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const client = useFleetConsoleClient();

  const query = useQuery(
    client.ListResourceRequests.query({
      filter: '', // TODO: b/396079336 add filtering
      orderBy: '', // TODO: b/397392558 add sorting
      pageSize: pagerCtx.options.defaultPageSize,
      pageToken: '', // TODO: b/396079903 add pagination
    }),
  );

  const [sortModel, setSortModel] = useState<GridSortModel>([
    { field: 'id', sort: 'asc' },
  ]);

  const columns: GridColDef[] = [
    {
      field: 'id',
      headerName: 'RR ID',
      flex: 1,
    },
    {
      field: 'resource_details',
      headerName: 'Resource Details',
      flex: 1,
    },
    {
      field: 'procurement_end_date',
      headerName: 'Procurement End Date',
      flex: 1,
    },
    {
      field: 'build_end_date',
      headerName: 'Build End Date',
      flex: 1,
    },
    {
      field: 'qa_end_date',
      headerName: 'QA End Date',
      flex: 1,
    },
    {
      field: 'config_end_date',
      headerName: 'Config End Date',
      flex: 1,
    },
  ];

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
        columns={columns}
        rows={rows}
        slots={{
          pagination: Pagination,
        }}
        slotProps={{
          pagination: {
            pagerCtx: pagerCtx,
            nextPageToken: '',
          },
        }}
        paginationMode="server"
        pageSizeOptions={pagerCtx.options.pageSizeOptions}
        rowCount={-1} // TODO: b/396079903 handle pagination
        paginationModel={{
          page: getCurrentPageIndex(pagerCtx),
          pageSize: pagerCtx.options.defaultPageSize,
        }}
        rowSelection={false}
        sortModel={sortModel}
        onSortModelChange={setSortModel}
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
