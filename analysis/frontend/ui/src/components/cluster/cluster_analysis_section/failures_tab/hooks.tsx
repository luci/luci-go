import { ParamKeyValuePair, useSearchParams } from 'react-router-dom';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { MetricName } from '@/tools/failures_tools';

export function useSelectedVariantGroupsParam(): [string[], (selectedVariantGroups: string[], replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const groupByParam = searchParams.get('groupBy') || '';
  let selectedVariantGroups: string[] = [];
  if (groupByParam) {
    selectedVariantGroups = groupByParam.split(',');
  }

  function updateSelectedVariantGroupsParam(selectedVariantGroups: string[], replace = false) {
    const params: ParamKeyValuePair[] = [];

    for (const [k, v] of searchParams.entries()) {
      if (k !== 'groupBy') {
        params.push([k, v]);
      }
    }

    params.push(['groupBy', selectedVariantGroups.join(',')]);

    setSearchParams(params, {
      replace,
    });
  }

  return [selectedVariantGroups, updateSelectedVariantGroupsParam];
}

export function useFilterToMetricParam(metrics: ProjectMetric[]): [ProjectMetric | undefined, (filterToMetric: ProjectMetric | undefined, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const filterToMetricParam = searchParams.get('filterToMetric') || '';

  const filterToMetric = metrics.find((metric) => filterToMetricParam === metric.metricId);

  function updateFilterToMetricParam(filterToMetric: ProjectMetric | undefined, replace = false) {
    const params: ParamKeyValuePair[] = [];
    for (const [k, v] of searchParams.entries()) {
      if (k !== 'filterToMetric') {
        params.push([k, v]);
      }
    }

    if (filterToMetric) {
      params.push(['filterToMetric', filterToMetric.metricId]);
    }

    setSearchParams(params, {
      replace,
    });
  }

  return [filterToMetric, updateFilterToMetricParam];
}

export function useOrderByParam(defaultMetricName: MetricName): [MetricName, (metricName: MetricName, replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const orderByParam = (searchParams.get('orderBy') as MetricName) || defaultMetricName;

  function updateOrderByParam(metricName: MetricName, replace = false) {
    const params: ParamKeyValuePair[] = [];
    for (const [k, v] of searchParams.entries()) {
      if (k !== 'orderBy') {
        params.push([k, v]);
      }
    }

    params.push(['orderBy', metricName]);

    setSearchParams(params, {
      replace,
    });
  }

  return [orderByParam, updateOrderByParam];
}
