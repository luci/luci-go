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

import { Box } from '@mui/material';
import { useMemo } from 'react';

import { ChartSeriesItem } from '@/crystal_ball/components';
import { Column, COMMON_MESSAGES } from '@/crystal_ball/constants';
import {
  EditorUiKeyPrefix,
  useFetchDashboardWidgetData,
  useExportToSheets,
} from '@/crystal_ball/hooks';
import { prepareExportData, UNKNOWN_DIMENSION } from '@/crystal_ball/utils';
import {
  BreakdownTableConfig_BreakdownAggregation,
  MeasurementFilterColumn,
  MeasurementFilterColumn_FilterScope,
  PerfChartWidget,
  PerfChartWidget_ChartType,
  PerfDashboardContent,
  PerfDataSpec,
  PerfFilter,
  PerfWidget,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { BreakdownTableChart } from './breakdown_table_chart';

/**
 * A widget that displays performance breakdown data.
 */
export interface BreakdownTableWidgetProps {
  widget: PerfChartWidget;
  dashboardName: string;
  widgetId: string;
  globalFilters?: readonly PerfFilter[];
  filterColumns: readonly MeasurementFilterColumn[];
  isLoadingFilterColumns?: boolean;
  dataSpecs?: { [key: string]: PerfDataSpec };
  onUpdate: (updatedWidget: PerfChartWidget) => void;
}

/**
 * A widget that displays performance breakdown data, handling data fetching
 * and configuration editing.
 */
export function BreakdownTableWidget({
  widget,
  widgetId,
  globalFilters,
  filterColumns,
  isLoadingFilterColumns,
  dataSpecs,
  onUpdate,
}: BreakdownTableWidgetProps): React.ReactElement {
  const currentAggregations = useMemo(() => {
    const config = widget.breakdownTableWidgetChartConfig;
    if (config?.aggregations?.length) {
      return config.aggregations;
    }
    return [
      BreakdownTableConfig_BreakdownAggregation.COUNT,
      BreakdownTableConfig_BreakdownAggregation.MIN,
      BreakdownTableConfig_BreakdownAggregation.MAX,
    ];
  }, [widget.breakdownTableWidgetChartConfig]);

  const hasAtpTestFilter = useMemo(() => {
    const allFilters = [
      ...(globalFilters ?? []),
      ...(widget.filters ?? []),
      ...(widget.series?.flatMap((s) => s.filters ?? []) ?? []),
    ];
    return allFilters.some(
      (f) =>
        f.column === Column.ATP_TEST_NAME &&
        f.textInput?.defaultValue?.values?.[0],
    );
  }, [globalFilters, widget.filters, widget.series]);

  const hasEmptyMetricField = useMemo(() => {
    return widget.series?.some((s) => !s.metricField);
  }, [widget.series]);

  const metricFilterColumns = useMemo(
    () =>
      filterColumns.filter(
        (c) =>
          c.applicableScopes?.includes(
            MeasurementFilterColumn_FilterScope.METRIC,
          ) ||
          (Array.isArray(c.applicableScopes) &&
            c.applicableScopes.includes('METRIC')),
      ),
    [filterColumns],
  );

  const fetchRequest = useMemo(
    () => ({
      dashboardContent: PerfDashboardContent.fromPartial({
        globalFilters: globalFilters ?? [],
        dataSpecs: dataSpecs ?? {},
        widgets: [
          PerfWidget.fromPartial({
            id: widgetId,
            chart: PerfChartWidget.fromPartial({
              ...widget,
              chartType: PerfChartWidget_ChartType.BREAKDOWN_TABLE,
              breakdownTableWidgetChartConfig: {
                aggregations: [...currentAggregations],
              },
            }),
          }),
        ],
      }),
      widgetId,
    }),
    [globalFilters, dataSpecs, widgetId, widget, currentAggregations],
  );

  const {
    data: responseData,
    isLoading,
    error,
  } = useFetchDashboardWidgetData(fetchRequest, {
    enabled:
      !!widgetId &&
      (widget.series?.length ?? 0) > 0 &&
      hasAtpTestFilter &&
      !hasEmptyMetricField,
  });

  const sections = responseData?.breakdownTableData?.sections ?? [];

  const { mutate: exportToSheets, isPending: isExporting } =
    useExportToSheets();

  const handleExport = () => {
    const activeDimension =
      widget.breakdownTableWidgetChartConfig?.defaultDimension ||
      sections[0]?.dimensionColumn ||
      UNKNOWN_DIMENSION;
    const activeSection =
      sections.find((s) => s.dimensionColumn === activeDimension) ??
      sections[0];

    if (!activeSection) return;

    const { title, values } = prepareExportData(
      activeSection,
      widget.series?.[0]?.metricField,
      globalFilters,
      widget.filters,
      widget.series?.[0]?.filters,
    );

    exportToSheets({ title, values });
  };

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <Box>
        <ChartSeriesItem
          uiStateOptions={{
            prefix: EditorUiKeyPrefix.BREAKDOWN_SERIES,
            key: widgetId,
          }}
          series={
            widget.series?.[0] ?? {
              displayName: '',
              metricField: '',
              dataSpecId: widget.dataSpecId,
              color: '',
              filters: [],
            }
          }
          onUpdate={(updatedItem) => {
            onUpdate({
              ...widget,
              series: [updatedItem],
            });
          }}
          onRemove={() => {
            onUpdate({
              ...widget,
              series: [],
            });
          }}
          dataSpecId={widget.dataSpecId}
          globalFilters={globalFilters}
          metricFilterColumns={metricFilterColumns}
          isLoadingColumns={isLoadingFilterColumns}
          hideColorPicker={true}
          hideVisibility={true}
          hideMultiSeriesActions={true}
          titlePlaceholder={COMMON_MESSAGES.ADD_FILTER_METRIC_SERIES}
        />
      </Box>
      <BreakdownTableChart
        sections={sections}
        isLoading={isLoading}
        error={error}
        currentAggregations={currentAggregations}
        hasAtpTestFilter={hasAtpTestFilter}
        hasEmptyMetricField={hasEmptyMetricField}
        onUpdateAggregations={(finalValues) => {
          onUpdate(
            PerfChartWidget.fromPartial({
              ...widget,
              chartType: PerfChartWidget_ChartType.BREAKDOWN_TABLE,
              breakdownTableWidgetChartConfig: {
                ...widget.breakdownTableWidgetChartConfig,
                aggregations: finalValues,
              },
            }),
          );
        }}
        hasSeries={!!widget.series?.[0]?.metricField}
        defaultDimension={
          widget.breakdownTableWidgetChartConfig?.defaultDimension
        }
        onUpdateDefaultDimension={(dimension) => {
          onUpdate(
            PerfChartWidget.fromPartial({
              ...widget,
              breakdownTableWidgetChartConfig: {
                ...widget.breakdownTableWidgetChartConfig,
                defaultDimension: dimension,
              },
            }),
          );
        }}
        onExport={handleExport}
        isExporting={isExporting}
      />
    </Box>
  );
}
