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
  MRT_PaginationState,
  MRT_Updater,
  MaterialReactTable,
} from 'material-react-table';
import { useCallback, useEffect, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { getErrorMessage } from '@/fleet/utils/errors';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics/track_leaf_route_page_view';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { COLUMNS } from './product_catalogue_columns';
import { useProductCatalogFilters } from './use_product_catalog_filters';

const Container = styled.div`
  margin: 24px;
`;

const DEFAULT_PAGE_SIZE = 25;
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

export const ProductCataloguePage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const client = useFleetConsoleClient();

  const { filterValues, aip160, onApplyFilter, isLoading } =
    useProductCatalogFilters(() => {
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
    });

  const query = useQuery({
    ...client.ListProductCatalogEntries.query({ filter: aip160 }),
    placeholderData: keepPreviousData,
  });

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

  const table = useFCDataTable({
    columns: COLUMNS,
    data: [...(query.data?.entries ?? [])],
    enablePagination: true,
    manualPagination: false,
    manualFiltering: false,
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
      <div
        css={{
          marginBottom: 24,
          width: '100%',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 28,
          borderRadius: 4,
        }}
      >
        <FilterBar
          filterCategoryDatas={Object.values(filterValues || {})}
          onApply={onApplyFilter}
          isLoading={isLoading}
        />
      </div>
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
