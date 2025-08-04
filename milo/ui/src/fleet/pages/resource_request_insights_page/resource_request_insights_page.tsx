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
import { useCallback, useEffect, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { FilterButton } from '@/fleet/components/filter_dropdown/filter_button';
import { FilterCategoryData } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { SelectedChip } from '@/fleet/components/filter_dropdown/selected_chip';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { fuzzySubstring } from '@/fleet/utils/fuzzy_sort';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { ResourceRequestTable } from './resource_requests_table';
import { RRI_COLUMNS } from './rri_columns';
import { RriSummaryHeader } from './rri_summary_header';
import {
  ResourceRequestInsightsOptionComponentProps,
  RriFilterKey,
  RriFilterOption,
  RriFilters,
  useRriFilters,
} from './use_rri_filters';

const Container = styled.div`
  margin: 24px;
`;

export const ResourceRequestListPage = () => {
  const { filterComponents, filterData, setFilters, getSelectedFilterLabel } =
    useRriFilters();

  const [currentFilters, setCurrentFilters] = useState<RriFilters | undefined>(
    filterData,
  );

  useEffect(() => setCurrentFilters(filterData), [filterData]);

  const clearSelections = useCallback(() => {
    setCurrentFilters(filterData);
  }, [setCurrentFilters, filterData]);

  const onApplyFilters = () => {
    setFilters(currentFilters);
  };

  const filterCategoryDatas: FilterCategoryData<ResourceRequestInsightsOptionComponentProps>[] =
    filterComponents.map((option) => {
      const label: string =
        RRI_COLUMNS.find((c) => c.id === option.value)?.gridColDef.headerName ??
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

  const getSelectedChipDropdownContent = (
    filterKey: RriFilterKey,
    searchQuery: string,
  ) => {
    const filterOption = filterComponents.find(
      (opt) => opt.value === filterKey,
    );
    if (!filterOption) {
      return undefined;
    }

    const OptionComponent = filterOption.optionsComponent;

    return (
      <OptionComponent
        searchQuery={searchQuery}
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
          // isLoading={query.isPending}
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
                dropdownContent={(searchQuery) =>
                  getSelectedChipDropdownContent(filterKey, searchQuery)
                }
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
      <ResourceRequestTable />
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
