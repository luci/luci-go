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
import { DateTime } from 'luxon';
import { useCallback, useEffect, useState } from 'react';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { FilterCategoryData } from '@/fleet/components/filter_dropdown/filter_dropdown';
import { LoggedInBoundary } from '@/fleet/components/logged_in_boundary';
import { FleetHelmet } from '@/fleet/layouts/fleet_helmet';
import { DateFilterValue } from '@/fleet/types';
import { fromLuxonDateTime, toLuxonDateTime } from '@/fleet/utils/dates';
import { fuzzySubstring } from '@/fleet/utils/fuzzy_sort';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { ResourceRequestTable } from './resource_requests_table';
import { RRI_COLUMNS } from './rri_columns';
import { RriSummaryHeader } from './rri_summary_header';
import {
  DateFilterData,
  RangeFilterData,
  RriFilterKey,
  RriFilterOption,
  RriFilters,
  useRriFilters,
} from './use_rri_filters';

const Container = styled.div`
  margin: 24px;
`;

const createDateFilterAdapter = (
  option: RriFilterOption,
  commonProps: Partial<FilterCategoryData<unknown>>,
  currentFilters: RriFilters | undefined,
  setCurrentFilters: (filters: RriFilters | undefined) => void,
): FilterCategoryData<unknown> => {
  const val = currentFilters?.[option.value] as DateFilterData | undefined;
  return {
    ...commonProps,
    value: option.value,
    label: commonProps.label || option.value,
    getChildrenSearchScore: commonProps.getChildrenSearchScore!,
    type: 'date',
    optionsComponentProps: {
      value: {
        min: toLuxonDateTime(val?.min)?.toJSDate(),
        max: toLuxonDateTime(val?.max)?.toJSDate(),
      },
      onChange: (newVal: DateFilterValue) => {
        setCurrentFilters({
          ...currentFilters,
          [option.value]: {
            min: fromLuxonDateTime(
              newVal.min ? DateTime.fromJSDate(newVal.min) : undefined,
            ),
            max: fromLuxonDateTime(
              newVal.max ? DateTime.fromJSDate(newVal.max) : undefined,
            ),
          },
        });
      },
    },
  };
};

const createRangeFilterAdapter = (
  option: RriFilterOption,
  commonProps: Partial<FilterCategoryData<unknown>>,
  currentFilters: RriFilters | undefined,
  setCurrentFilters: (filters: RriFilters | undefined) => void,
): FilterCategoryData<unknown> => {
  const val = currentFilters?.[option.value] as RangeFilterData | undefined;
  return {
    ...commonProps,
    value: option.value,
    label: commonProps.label || option.value,
    getChildrenSearchScore: commonProps.getChildrenSearchScore!,
    type: 'range',
    optionsComponentProps: {
      value: {
        min: val?.min,
        max: val?.max,
      },
      onChange: (newVal: { min?: number; max?: number }) => {
        setCurrentFilters({
          ...currentFilters,
          [option.value]: {
            min: newVal.min,
            max: newVal.max,
          },
        });
      },
    },
  };
};

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

  const filterCategoryDatas: FilterCategoryData<unknown>[] =
    filterComponents.map((option) => {
      const label: string =
        RRI_COLUMNS.find((c) => c.id === option.value)?.gridColDef.headerName ??
        option.value;

      const commonProps = {
        label: label,
        value: option.value,
        type: option.type,
        getChildrenSearchScore: (searchQuery: string) => {
          const typedOption = option as RriFilterOption;
          const childrenScore = typedOption.getChildrenSearchScore
            ? typedOption.getChildrenSearchScore(searchQuery)
            : 0;
          const [score] = fuzzySubstring(searchQuery, label);
          return Math.max(score, childrenScore);
        },
      };

      if (option.type === 'date') {
        return createDateFilterAdapter(
          option,
          commonProps,
          currentFilters,
          setCurrentFilters,
        );
      }

      if (option.type === 'range') {
        return createRangeFilterAdapter(
          option,
          commonProps,
          currentFilters,
          setCurrentFilters,
        );
      }

      return {
        ...commonProps,
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        optionsComponent: option.optionsComponent as any,
        optionsComponentProps: {
          option: option,
          onApply: onApplyFilters,
          filters: currentFilters,
          onFiltersChange: setCurrentFilters,
          onClose: clearSelections,
        },
      };
    });

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
        <FilterBar
          filterCategoryDatas={filterCategoryDatas}
          selectedOptions={Object.keys(filterData ?? {}).filter(
            (k) => filterData?.[k as RriFilterKey] !== undefined,
          )}
          onApply={onApplyFilters}
          getChipLabel={(o) => {
            const val = filterData?.[o.value as RriFilterKey];
            return val
              ? getSelectedFilterLabel(o.value as RriFilterKey, val)
              : '';
          }}
          onChipDeleted={(o) => {
            const newFilters = { ...filterData };

            delete newFilters[o.value as RriFilterKey];
            setFilters(newFilters);
          }}
          searchPlaceholder='Add a filter (e.g. "rr_id" or "status")'
        />
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
        fallbackRender={({ error }) => (
          <LoggedInBoundary>
            <div css={{ padding: 24 }}>
              <>{error instanceof Error ? error.message : String(error)}</>
            </div>
          </LoggedInBoundary>
        )}
      >
        <LoggedInBoundary>
          <ResourceRequestListPage />
        </LoggedInBoundary>
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
