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
import GridViewIcon from '@mui/icons-material/GridView';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import TableRowsIcon from '@mui/icons-material/TableRows';
import {
  Box,
  IconButton,
  Link,
  Tooltip,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import {
  MaterialReactTable,
  MRT_PaginationState,
  MRT_Updater,
} from 'material-react-table';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useFCDataTable } from '@/fleet/components/fc_data_table/use_fc_data_table';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useMrtSortingState } from '@/fleet/hooks/use_mrt_sorting_state';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { WarningNotifications } from '@/fleet/utils/use_warnings';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics/track_leaf_route_page_view';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { ProductCatalogueCardView } from './product_catalogue_card_view';
import { COLUMNS } from './product_catalogue_columns';
import {
  useProductCatalogFilters,
  FILTERS,
} from './use_product_catalog_filters';
import {
  ProductCatalogTab,
  useProductCatalogTabs,
} from './use_product_catalog_tabs';

const Container = styled.div`
  margin: 24px;
`;

const DEFAULT_PAGE_SIZE = 50;

const ProductCatalogueHeader = () => {
  return (
    <Box
      sx={{
        p: 3,
        mb: 3,
        borderRadius: 1,
        border: `1px solid ${colors.grey[300]}`,
        background: `linear-gradient(135deg, ${colors.grey[50]} 0%, ${colors.white} 100%)`,
      }}
    >
      <Typography
        variant="h5"
        sx={{ fontWeight: 600, mb: 1, color: colors.grey[900] }}
      >
        Product Catalog
      </Typography>
      <Typography variant="body2" sx={{ color: colors.grey[700], mb: 3 }}>
        The Product Catalog lists all hardware configurations and models
        available in the fleet. Use this catalog to browse specifications and
        verify existing capacity before requesting new hardware.
      </Typography>

      <Box sx={{ display: 'flex', gap: 3, flexWrap: 'wrap' }}>
        <Link
          href="http://go/ineedhw"
          target="_blank"
          rel="noopener noreferrer"
          sx={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 0.5,
            fontSize: '0.875rem',
            fontWeight: 500,
            textDecoration: 'none',
            '&:hover': { textDecoration: 'underline' },
          }}
        >
          Resource Request Program (go/ineedhw)
          <OpenInNewIcon sx={{ fontSize: 16 }} />
        </Link>
        <Link
          href="http://go/fcon-user-guide#product-catalog"
          target="_blank"
          rel="noopener noreferrer"
          sx={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 0.5,
            fontSize: '0.875rem',
            fontWeight: 500,
            textDecoration: 'none',
            '&:hover': { textDecoration: 'underline' },
          }}
        >
          Product catalog documentation
          <OpenInNewIcon sx={{ fontSize: 16 }} />
        </Link>
      </Box>
    </Box>
  );
};

interface ProductCatalogueTabPageContentProps {
  selectedTab: ProductCatalogTab;
  columns: typeof COLUMNS;
}

const ProductCatalogueTabPageContent = ({
  selectedTab,
  columns,
}: ProductCatalogueTabPageContentProps) => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const client = useFleetConsoleClient();

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

  const { filterValues, aip160, onApplyFilter, isLoading, warnings } =
    useProductCatalogFilters(selectedTab, () => {
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
    });

  const query = useQuery({
    ...client.ListProductCatalogEntries.query({ filter: aip160 }),
    placeholderData: keepPreviousData,
  });

  const onPaginationChange = useCallback(
    (updater: MRT_Updater<MRT_PaginationState>) => {
      if (!query.data) {
        return;
      }

      const next =
        typeof updater === 'function' ? updater(pagination) : updater;
      setPagination(next);
      if (next.pageSize !== pagination.pageSize) {
        setSearchParams((prevParams) => {
          const nextParams = new URLSearchParams(prevParams);
          nextParams.set('pageSize', String(next.pageSize));
          return nextParams;
        });
      }
    },
    [query.data, pagination, setSearchParams],
  );

  const data = (query.data?.entries ?? []) as ProductCatalogEntry[];

  const mappedFilterValues = useMemo(
    () =>
      Object.fromEntries(
        Object.entries(filterValues || {}).map(([key, value]) => {
          const entry = Object.entries(FILTERS).find(
            ([_, config]) => `"${config.filterKey}"` === key,
          );
          return [entry ? entry[0] : key, value];
        }),
      ),
    [filterValues],
  );

  const table = useFCDataTable({
    columns: columns,
    data: data,
    filterValues: mappedFilterValues,
    enablePagination: true,
    manualPagination: false,
    manualFiltering: true,
    manualSorting: false,
    autoResetPageIndex: false,
    error: query.error
      ? getErrorMessage(query.error, 'get product catalog')
      : undefined,
    onPaginationChange: onPaginationChange,
    onSortingChange: onSortingChange,
    state: {
      pagination,
      sorting,
      isLoading: query.isLoading,
      showProgressBars: query.isFetching,
    },
    renderEmptyRowsFallback: () => (
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          p: 4,
        }}
      >
        <Typography color="text.secondary">No products found</Typography>
      </Box>
    ),
  });

  const view = searchParams.get('view') === 'card' ? 'card' : 'table';

  const handleViewChange = (newView: string) => {
    setSearchParams((prevParams) => {
      const nextParams = new URLSearchParams(prevParams);
      nextParams.set('view', newView);
      nextParams.delete('pageSize');
      return nextParams;
    });
  };

  return (
    <>
      <WarningNotifications warnings={warnings} />
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
          searchPlaceholder='Add a filter (e.g. "gpn:1234567")'
        />
        <Tooltip
          title={
            view === 'card' ? 'Switch to Table View' : 'Switch to Card View'
          }
        >
          <IconButton
            onClick={() => handleViewChange(view === 'card' ? 'table' : 'card')}
            sx={{
              border: `1px solid ${colors.grey[300]}`,
              borderRadius: '8px',
              '&:hover': {
                backgroundColor: colors.grey[50],
                color: colors.blue[700],
                borderColor: colors.blue[200],
              },
            }}
          >
            {view === 'card' ? <TableRowsIcon /> : <GridViewIcon />}
          </IconButton>
        </Tooltip>
      </div>
      {view === 'card' ? (
        <ProductCatalogueCardView
          entries={(query.data?.entries ?? []).slice(
            pagination.pageIndex * pagination.pageSize,
            (pagination.pageIndex + 1) * pagination.pageSize,
          )}
          isLoading={query.isLoading}
          pagination={pagination}
          onPaginationChange={onPaginationChange}
          totalCount={query.data?.entries?.length ?? 0}
        />
      ) : (
        <MaterialReactTable table={table} />
      )}
    </>
  );
};

const ALL_TAB_COLUMN_KEYS: readonly (keyof ProductCatalogEntry)[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
  'productType',
];

const ANDROID_TESTBED_COLUMN_KEYS: readonly (keyof ProductCatalogEntry)[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

const HARDWARE_COLUMN_KEYS: readonly (keyof ProductCatalogEntry)[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

const OS_TESTBED_COLUMN_KEYS: readonly (keyof ProductCatalogEntry)[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

const PERIPHERALS_COLUMN_KEYS: readonly (keyof ProductCatalogEntry)[] = [
  'productCatalogId',
  'productName',
  'gpn',
  'descriptiveName',
  'resourceType',
  'fleetPlmStatus',
  'r11n',
  'numberOfDevicesPerRack',
  'unitCost',
];

const TAB_COLUMN_KEYS: Record<
  ProductCatalogTab | string,
  readonly (keyof ProductCatalogEntry)[]
> = {
  [ProductCatalogTab.ALL]: ALL_TAB_COLUMN_KEYS,
  [ProductCatalogTab.ANDROID_TESTBED]: ANDROID_TESTBED_COLUMN_KEYS,
  [ProductCatalogTab.HARDWARE]: HARDWARE_COLUMN_KEYS,
  [ProductCatalogTab.OS_TESTBED]: OS_TESTBED_COLUMN_KEYS,
  [ProductCatalogTab.PERIPHERALS]: PERIPHERALS_COLUMN_KEYS,
};

const getColumnsForTab = (tab: ProductCatalogTab | string) => {
  const keys = TAB_COLUMN_KEYS[tab];
  if (!keys) {
    throw new Error(`Unknown product catalog tab: "${tab}"`);
  }
  return COLUMNS.filter(
    (column) =>
      column.accessorKey !== undefined &&
      keys.includes(column.accessorKey as keyof ProductCatalogEntry),
  ) as typeof COLUMNS;
};

export const ProductCataloguePage = () => {
  const { tabs, selectedTab, handleTabChange } = useProductCatalogTabs();
  const columns = useMemo(() => getColumnsForTab(selectedTab), [selectedTab]);

  return (
    <Container>
      <ProductCatalogueHeader />
      {tabs && tabs.length > 0 && selectedTab && (
        <Box sx={{ borderBottom: 1, borderColor: 'divider', marginBottom: 2 }}>
          <Tabs value={selectedTab} onChange={handleTabChange}>
            {tabs.map((tab) => (
              <Tab key={tab.value} label={tab.value} value={tab.value} />
            ))}
          </Tabs>
        </Box>
      )}
      <ProductCatalogueTabPageContent
        key={selectedTab}
        selectedTab={selectedTab}
        columns={columns}
      />
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
