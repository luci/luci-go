// Copyright 2026 The LUCI Authors.
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

import styled from '@emotion/styled';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import {
  MaterialReactTable,
  MRT_PaginationState,
  MRT_Updater,
} from 'material-react-table';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics/track_leaf_route_page_view';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { COLUMNS } from './product_catalogue_columns';

const Container = styled.div`
  margin: 24px;
`;

const DEFAULT_PAGE_SIZE = 25;
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

export const ProductCataloguePage = () => {
  const client = useFleetConsoleClient();
  const query = useQuery({
    ...client.ListProductCatalogEntries.query({}),
    placeholderData: keepPreviousData,
  });

  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const pageSize =
    parseInt(searchParams.get('pageSize') || String(DEFAULT_PAGE_SIZE), 10) ||
    DEFAULT_PAGE_SIZE;

  const [pagination, setPagination] = useState<MRT_PaginationState>(() => ({
    pageIndex: 0,
    pageSize: pageSize,
  }));

  useEffect(() => {
    setPagination((prev) => {
      if (prev.pageSize === pageSize) return prev;
      return { pageIndex: 0, pageSize };
    });
  }, [pageSize]);

  const [sorting, onSortingChange] = useMrtSortingState();

  const onPaginationChange = useCallback(
    (updater: MRT_Updater<MRT_PaginationState>) => {
      if (!query.data) {
        return;
      }

      setPagination((prev) => {
        const next = typeof updater === 'function' ? updater(prev) : updater;
        if (next.pageSize !== prev.pageSize) {
          setSearchParams((prevParams) => {
            const nextParams = new URLSearchParams(prevParams);
            nextParams.set('pageSize', String(next.pageSize));
            return nextParams;
          });
        }
        return next;
      });
    },
    [query.data, setSearchParams],
  );

  const data = useMemo(() => [...(query.data?.entries ?? [])], [query.data]);

  const table = useFCDataTable({
    columns: COLUMNS,
    data,
    enablePagination: true,
    manualPagination: false,
    onSortingChange: onSortingChange,
    onPaginationChange: onPaginationChange,
    autoResetPageIndex: false,
    muiPaginationProps: {
      rowsPerPageOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    },
    state: {
      pagination,
      sorting,
      isLoading: query.isLoading,
      showProgressBars: query.isFetching,
      showAlertBanner: query.isError,
    },
    muiToolbarAlertBannerProps: query.isError
      ? {
          color: 'error',
          children: getErrorMessage(query.error, 'get product catalog'),
        }
      : undefined,
  });

  return (
    <Container>
      <MaterialReactTable table={table} />
    </Container>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-product-catalog-list">
      <FleetHelmet pageTitle="Product Catalog" />
      <RecoverableErrorBoundary
        fallbackRender={({ error }) => (
          <LoggedInBoundary>
            <div css={{ padding: 24 }}>
              <>{error instanceof Error ? error.message : String(error)}</>
            </div>
          </LoggedInBoundary>
        )}
      >
        <LoggedInBoundary>
          <ProductCataloguePage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
