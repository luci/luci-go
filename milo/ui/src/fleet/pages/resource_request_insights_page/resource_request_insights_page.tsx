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

import styled from '@emotion/styled';
import { Alert, CircularProgress } from '@mui/material';
import {
  GridColumnVisibilityModel,
  GridSortItem,
  GridSortModel,
} from '@mui/x-data-grid';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageSize,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { Pagination } from '@/fleet/components/device_table/pagination';
import { useParamsAndLocalStorage } from '@/fleet/components/device_table/use_params_and_local_storage';
import { FilterButton } from '@/fleet/components/filter_dropdown/filter_button';
import { FilterCategoryData } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { SelectedChip } from '@/fleet/components/filter_dropdown/selected_chip';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { RriTableToolbar } from '@/fleet/components/resource_request_insights/rri_table_toolbar';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY } from '@/fleet/constants/local_storage_keys';
import { COLUMNS_PARAM_KEY } from '@/fleet/constants/param_keys';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { fuzzySubstring } from '@/fleet/utils/fuzzy_sort';
import { getVisibilityModel } from '@/fleet/utils/search_param';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import {
  ColumnDescriptor,
  DEFAULT_SORT_COLUMN,
  getColumnByField,
  rriColumns,
} from './rri_columns';
import { RriSummaryHeader } from './rri_summary_header';
import {
  ResourceRequestInsightsOptionComponentProps,
  RriFilterKey,
  RriFilterOption,
  RriFilters,
  useRriFilters,
} from './use_rri_filters';

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50];
const DEFAULT_PAGE_SIZE = 25;

const Container = styled.div`
  margin: 24px;
`;

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

export const ResourceRequestListPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam, updateOrderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const sortModel = getSortModelFromOrderByParam(orderByParam);

  const {
    filterComponents,
    filterData,
    aipString,
    setFilters,
    getSelectedFilterLabel,
  } = useRriFilters();

  const [currentFilters, setCurrentFilters] = useState<RriFilters | undefined>(
    filterData,
  );

  const [visibleColumns, setVisibleColumns] = useParamsAndLocalStorage(
    COLUMNS_PARAM_KEY,
    RRI_DEVICES_COLUMNS_LOCAL_STORAGE_KEY,
    rriColumns
      .filter((column: ColumnDescriptor) => column.isDefault)
      .map((column: ColumnDescriptor) => column.gridColDef.field),
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
    return rriColumns.map((column) => column.gridColDef);
  }, []);

  useEffect(() => setCurrentFilters(filterData), [filterData]);

  const clearSelections = useCallback(() => {
    setCurrentFilters(filterData);
  }, [setCurrentFilters, filterData]);

  const onApplyFilters = () => {
    setFilters(currentFilters);

    // Clear out all the page tokens when the filter changes.
    // An AIP-158 page token is only valid for the filter
    // option that generated it.
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  const filterCategoryDatas: FilterCategoryData<ResourceRequestInsightsOptionComponentProps>[] =
    filterComponents.map((option) => {
      const label: string =
        rriColumns.find((c) => c.id === option.value)?.gridColDef.headerName ??
        option.value;

      return {
        label: label,
        value: option.value,
        getSearchScore: (searchQuery: string) => {
          const typedOption = option as RriFilterOption;
          const childrenScore = typedOption.getChildrenSearchScore
            ? typedOption.getChildrenSearchScore(searchQuery)
            : 0;
          const [score, matches] = fuzzySubstring(searchQuery, label);
          return { score: Math.max(score, childrenScore), matches: matches };
        },
        optionsComponent: option.optionsComponent,
        optionsComponentProps: {
          option: option,
          onApply: onApplyFilters,
          filters: currentFilters,
          onFiltersChange: setCurrentFilters,
          onClose: clearSelections,
        },
      } satisfies FilterCategoryData<ResourceRequestInsightsOptionComponentProps>;
    });

  const handleSortModelChange = (newSortModel: GridSortModel) => {
    updateOrderByParam(getOrderByParamFromSortModel(newSortModel));
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  const client = useFleetConsoleClient();

  const query = useQuery(
    client.ListResourceRequests.query({
      filter: aipString,
      orderBy: getOrderByDto(sortModel),
      pageSize: getPageSize(pagerCtx, searchParams),
      pageToken: getPageToken(pagerCtx, searchParams),
    }),
  );

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

  const rows: Record<string, string>[] = query.data.resourceRequests.map(
    (resourceRequest) => {
      const row: Record<string, string> = {};
      for (const column of rriColumns) {
        row[column.gridColDef.field] = column.valueGetter(resourceRequest);
      }
      return row;
    },
  );

  const getSelectedChipDropdownContent = (filterKey: RriFilterKey) => {
    const filterOption = filterComponents.find(
      (opt) => opt.value === filterKey,
    );
    if (!filterOption) {
      return undefined;
    }

    const OptionComponent = filterOption.optionsComponent;

    return (
      <OptionComponent
        searchQuery={''}
        optionComponentProps={{
          filters: currentFilters,
          onClose: clearSelections,
          onFiltersChange: setCurrentFilters,
          option: filterComponents.find((opt) => opt.value === filterKey)!,
          onApply: onApplyFilters,
        }}
      />
    );
  };

  const getFilterBar = () => {
    return (
      <div
        css={{
          marginTop: 24,
          width: '100%',
          display: 'flex',
          justifyContent: 'flex-start',
          alignItems: 'center',
          gap: 8,
          borderRadius: 4,
        }}
      >
        <FilterButton
          filterOptions={filterCategoryDatas}
          onApply={onApplyFilters}
          isLoading={query.isPending}
        />
        {(
          Object.entries(filterData ?? {}) as [
            RriFilterKey,
            RriFilters[RriFilterKey],
          ][]
        ).map(
          ([filterKey, filterValue]) =>
            filterValue && (
              <SelectedChip
                key={filterKey}
                dropdownContent={getSelectedChipDropdownContent(filterKey)}
                label={getSelectedFilterLabel(filterKey, filterValue)}
                onApply={() => {
                  onApplyFilters();
                }}
                onDelete={() => {
                  setFilters({
                    ...currentFilters,
                    [filterKey]: undefined,
                  });
                }}
              />
            ),
        )}
      </div>
    );
  };

  return (
    <Container>
      <RriSummaryHeader />
      {getFilterBar()}

      {/* TODO: this piece of code is similar to data_table.tsx and could probably be separated to a shared component */}
      <div
        css={{
          borderRadius: 4,
          marginTop: 24,
        }}
      >
        <StyledGrid
          columns={columns}
          rows={rows}
          slots={{
            pagination: Pagination,
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
            rriColumns.map(
              (column: ColumnDescriptor) => column.gridColDef.field,
            ),
            visibleColumns,
          )}
          onColumnVisibilityModelChange={onColumnVisibilityModelChange}
        />
      </div>
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
