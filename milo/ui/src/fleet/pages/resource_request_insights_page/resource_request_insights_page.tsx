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
import {
  Alert,
  Checkbox,
  CircularProgress,
  MenuItem,
  MenuList,
} from '@mui/material';
import { GridColDef, GridSortItem, GridSortModel } from '@mui/x-data-grid';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  emptyPageTokenUpdater,
  getCurrentPageIndex,
  getPageToken,
  usePagerContext,
} from '@/common/components/params_pager';
import { Pagination } from '@/fleet/components/device_table/pagination';
import { FilterButton } from '@/fleet/components/filter_dropdown/filter_button';
import {
  FilterCategoryData,
  OptionComponent,
} from '@/fleet/components/filter_dropdown/filter_dropdown';
import {
  filtersUpdater,
  getFilters,
  GetFiltersResult,
} from '@/fleet/components/filter_dropdown/search_param_utils/search_param_utils';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { StyledGrid } from '@/fleet/components/styled_data_grid';
import { useOrderByParam } from '@/fleet/hooks/order_by';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { SelectedOptions } from '@/fleet/types';
import { toIsoString } from '@/fleet/utils/dates';
import { fuzzySubstring } from '@/fleet/utils/fuzzy_sort';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

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

const mapFulfillmentStatus = (
  fulfillmentStatus: ResourceRequest_Status | undefined,
): string => {
  if (fulfillmentStatus === undefined) return '';
  switch (fulfillmentStatus) {
    case ResourceRequest_Status.NOT_STARTED:
      return 'Not Started';
    case ResourceRequest_Status.IN_PROGRESS:
      return 'In Progress';
    case ResourceRequest_Status.COMPLETED:
      return 'Completed';
    default:
      return '';
  }
};

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
      mapFulfillmentStatus(rr.fulfillmentStatus),
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
] as const;

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

interface ResourceRequestInsightsOptionComponentProps {
  onSelectedOptionsChange: (x: SelectedOptions) => void;
}
interface RriFilterOption {
  label: string;
  value: string;
  optionsComponent: OptionComponent<ResourceRequestInsightsOptionComponentProps>;
}

export const ResourceRequestListPage = () => {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const [orderByParam, updateOrderByParam] = useOrderByParam();
  const pagerCtx = usePagerContext({
    pageSizeOptions: DEFAULT_PAGE_SIZE_OPTIONS,
    defaultPageSize: DEFAULT_PAGE_SIZE,
  });

  const sortModel = getSortModelFromOrderByParam(orderByParam);

  // Hardcoding filter options
  const [selectedOptions, setSelectedOptions] = useState<GetFiltersResult>(
    getFilters(searchParams),
  );

  const isSelected = (category: string, value: string) => {
    if (!selectedOptions.filters) return false;
    if (!selectedOptions.filters[category]) return false;
    return selectedOptions.filters[category].includes(value);
  };

  const flipOption = (category: string, value: string) => {
    let newSelectedOptions = selectedOptions.filters;

    if (!newSelectedOptions) {
      newSelectedOptions = { [category]: [value] };
    } else {
      if (isSelected(category, value)) {
        newSelectedOptions[category] = newSelectedOptions[category].filter(
          (v) => v !== value,
        );
      } else {
        newSelectedOptions[category] = [
          ...(newSelectedOptions[category] ?? []),
          value,
        ];
      }
    }

    onSelectedOptionsChange(newSelectedOptions);
  };

  const filterOpts: RriFilterOption[] = [
    {
      label: 'RR ID',
      value: 'rr_id',
      optionsComponent: () => (
        <>
          <MenuList>
            {['filter 1', 'filter 2', 'filter 3'].map((filterName) => (
              <MenuItem
                key={filterName}
                selected={isSelected('rr_id', filterName)}
                onClick={() => flipOption('rr_id', filterName)}
              >
                <Checkbox
                  sx={{
                    padding: 0,
                    marginRight: '13px',
                  }}
                  size="small"
                  checked={isSelected('rr_id', filterName)}
                  tabIndex={-1}
                />
                {filterName}
              </MenuItem>
            ))}
          </MenuList>
        </>
      ),
    },
    {
      label: 'Resource Details',
      value: 'resource_details',
      optionsComponent: () => <h1>Resource Details</h1>,
    },
  ];

  const getFilterCategoryDatas =
    (): FilterCategoryData<ResourceRequestInsightsOptionComponentProps>[] => {
      return filterOpts.map((option) => {
        return {
          label: option.label,
          value: option.value,
          getSearchScore: (searchQuery: string) => {
            const [score, matches] = fuzzySubstring(searchQuery, option.label);
            return { score: score, matches: matches };
          },
          optionsComponent: option.optionsComponent,
          optionsComponentProps: {
            onSelectedOptionsChange: onSelectedOptionsChange,
          },
        };
      });
    };

  const onSelectedOptionsChange = (newSelectedOptions: SelectedOptions) => {
    setSelectedOptions({ filters: newSelectedOptions, error: undefined });
    setSearchParams(filtersUpdater(newSelectedOptions));

    // Clear out all the page tokens when the filter changes.
    // An AIP-158 page token is only valid for the filter
    // option that generated it.
    setSearchParams(emptyPageTokenUpdater(pagerCtx));
  };

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

  if (selectedOptions.error || query.isError) {
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

  return (
    <Container>
      <div
        css={{
          marginTop: 24,
          width: '100%',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 28,
          borderRadius: 4,
        }}
      >
        <FilterButton
          filterOptions={getFilterCategoryDatas()}
          onApply={() => {}}
          isLoading={query.isLoading}
        />
      </div>

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
