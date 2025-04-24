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
import { GridColDef, GridSortItem, GridSortModel } from '@mui/x-data-grid';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { Pagination } from '@/fleet/components/device_table/pagination';
import { CustomSelectedChip } from '@/fleet/components/filter_dropdown/custom_selected_chip';
import { FilterButton } from '@/fleet/components/filter_dropdown/filter_button';
import { FilterCategoryData } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { toIsoString } from '@/fleet/utils/dates';
import { fuzzySubstring } from '@/fleet/utils/fuzzy_sort';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { fulfillmentStatusDisplayValueMap } from './fulfillment_status';
import { RriSummaryHeader } from './rri_summary_header';
import {
  filterOpts,
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

interface ColumnDescriptor {
  id: string;
  gridColDef: GridColDef;
  valueGetter: (rr: ResourceRequest) => string;
}

const columns = [
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
    id: 'expected_eta',
    gridColDef: {
      field: 'expected_eta',
      headerName: 'Expected ETA',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.expectedEta),
  },
  {
    id: 'fulfillment_status',
    gridColDef: {
      field: 'fulfillment_status',
      headerName: 'Fulfillment Status',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) =>
      rr.fulfillmentStatus !== undefined
        ? fulfillmentStatusDisplayValueMap[
            ResourceRequest_Status[
              rr.fulfillmentStatus
            ] as keyof typeof ResourceRequest_Status
          ]
        : '',
  },
  {
    id: 'material_sourcing_target_delivery_date',
    gridColDef: {
      field: 'material_sourcing_target_delivery_date',
      headerName: 'Material Sourcing Target Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.procurementEndDate),
  },
  {
    id: 'build_target_delivery_date',
    gridColDef: {
      field: 'build_target_delivery_date',
      headerName: 'Build Target Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.buildEndDate),
  },
  {
    id: 'qa_target_delivery_date',
    gridColDef: {
      field: 'qa_target_delivery_date',
      headerName: 'QA Target Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.qaEndDate),
  },
  {
    id: 'config_target_delivery_date',
    gridColDef: {
      field: 'config_target_delivery_date',
      headerName: 'Config Target Delivery Date',
      flex: 1,
    },
    valueGetter: (rr: ResourceRequest) => toIsoString(rr.configEndDate),
  },
] as const satisfies readonly ColumnDescriptor[];

export type ResourceRequestColumnKey = (typeof columns)[number]['id'];

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

  const [filters, aipString, setFilters, getSelectedFilterLabel] =
    useRriFilters();

  const [currentFilters, setCurrentFilters] = useState<RriFilters | undefined>(
    filters,
  );

  useEffect(() => setCurrentFilters(filters), [filters]);

  const clearSelections = useCallback(() => {
    setCurrentFilters(filters);
  }, [setCurrentFilters, filters]);

  const onApplyFilters = () => {
    setFilters(currentFilters);

    // Clear out all the page tokens when the filter changes.
    // An AIP-158 page token is only valid for the filter
    // option that generated it.
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

  const filterCategoryDatas: FilterCategoryData<ResourceRequestInsightsOptionComponentProps>[] =
    filterOpts.map((option) => {
      return {
        label: option.label,
        value: option.value,
        getSearchScore: (searchQuery: string) => {
          const typedOption = option as RriFilterOption;
          const childrenScore = typedOption.getChildrenSearchScore
            ? typedOption.getChildrenSearchScore(searchQuery)
            : 0;
          const [score, matches] = fuzzySubstring(searchQuery, option.label);
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
    (resourceRequest) => {
      const row: Record<string, string> = {};
      for (const column of columns) {
        row[column.gridColDef.field] = column.valueGetter(resourceRequest);
      }
      return row;
    },
  );

  const getSelectedChipDropdownContent = (filterKey: RriFilterKey) => {
    const filterOption = filterOpts.find((opt) => opt.value === filterKey);
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
          option: filterOpts.find((opt) => opt.value === filterKey)!,
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
        {(
          Object.entries(filters ?? {}) as [
            RriFilterKey,
            RriFilters[RriFilterKey],
          ][]
        ).map(
          ([filterKey, filterValue]) =>
            filterValue && (
              <CustomSelectedChip
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
        <FilterButton
          filterOptions={filterCategoryDatas}
          onApply={onApplyFilters}
          isLoading={query.isLoading}
        />
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
