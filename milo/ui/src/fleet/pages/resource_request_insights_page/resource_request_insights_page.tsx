import { GridColDef, GridSortModel } from '@mui/x-data-grid';
import { useState } from 'react';
import { Helmet } from 'react-helmet';

import bassFavicon from '@/common/assets/favicons/bass-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  getCurrentPageIndex,
  usePagerContext,
} from '@/common/components/params_pager';
import { StyledGrid } from '@/fleet/components/data_table/styled_data_grid';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

export const ResourceRequestListPage = () => {
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

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

  const rows = [
    {
      id: '1',
      resource_details: 'Macbook Pro 8GB',
      procurement_end_date: '2025-04-01',
      build_end_date: '2025-04-15',
      qa_end_date: '2025-04-28',
      config_end_date: '2025-05-12',
    },
    {
      id: '2',
      resource_details: 'Macbook Pro 16GB',
      procurement_end_date: '2025-04-12',
      build_end_date: '2025-04-21',
      qa_end_date: '2025-05-08',
      config_end_date: '2025-05-28',
    },
  ];

  return (
    <div
      css={{
        margin: '24px',
      }}
    >
      <StyledGrid
        columns={columns}
        rows={rows}
        paginationMode="server"
        pageSizeOptions={pagerCtx.options.pageSizeOptions}
        rowCount={rows.length} // TODO: handle pagination when we fetch data from backend
        paginationModel={{
          page: getCurrentPageIndex(pagerCtx),
          pageSize: pagerCtx.options.defaultPageSize,
        }}
        rowSelection={false}
        sortModel={sortModel}
        onSortModelChange={setSortModel}
      />
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-resource-request-list">
      <Helmet>
        <title>Streamlined Fleet UI</title>
        <link rel="icon" href={bassFavicon} />
      </Helmet>
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
