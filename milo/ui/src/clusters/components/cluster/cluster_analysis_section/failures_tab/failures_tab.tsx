// Copyright 2022 The LUCI Authors.
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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import TabPanel from '@mui/lab/TabPanel';
import { SelectChangeEvent } from '@mui/material/Select';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useContext, useEffect, useState } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useFetchClusterFailures from '@/clusters/hooks/use_fetch_cluster_failures';
import useFetchMetrics from '@/clusters/hooks/use_fetch_metrics';
import {
  countFailures,
  countDistictVariantValues,
  defaultImpactFilter,
  FailureGroup,
  groupAndCountFailures,
  ImpactFilter,
  ImpactFilters,
  MetricName,
  sortFailureGroups,
  VariantGroup,
} from '@/clusters/tools/failures_tools';
import { ProjectMetric } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';

import { ClusterContext } from '../../cluster_context';

import FailuresTableFilter from './failures_table_filter/failures_table_filter';
import FailuresTableGroup from './failures_table_group/failures_table_group';
import FailuresTableHead from './failures_table_head/failures_table_head';
import {
  useFilterToMetricParam,
  useOrderByParam,
  useSelectedVariantGroupsParam,
} from './hooks';

interface Props {
  // The name of the tab.
  value: string;
}

const FailuresTab = ({ value }: Props) => {
  const { project } = useContext(ClusterContext);
  const {
    isLoading: isLoading,
    data: metrics,
    error: error,
  } = useFetchMetrics(project);

  return (
    <TabPanel value={value}>
      {error && <LoadErrorAlert entityName="metrics" error={error} />}
      {isLoading && <CentralizedProgress />}
      {metrics !== undefined && <FailuresTable metrics={metrics} />}
    </TabPanel>
  );
};

interface TableProps {
  metrics: ProjectMetric[];
}

const FailuresTable = ({ metrics }: TableProps) => {
  const {
    project,
    algorithm: clusterAlgorithm,
    id: clusterId,
  } = useContext(ClusterContext);

  // This state should be kept outside the tab to avoid state loss
  // whenever the tab is not visible, as the tab's contents are
  // unmounted when it is not visible.
  const [groups, setGroups] = useState<FailureGroup[]>([]);
  const [variantGroups, setVariantGroups] = useState<VariantGroup[]>([]);

  const [filterToMetric, setFilterToMetric] = useFilterToMetricParam(metrics);
  const [impactFilter, setImpactFilter] =
    useState<ImpactFilter>(defaultImpactFilter);
  const [selectedVariantGroups, setSelectedVariantGroups] =
    useSelectedVariantGroupsParam();

  const [sortMetric, setCurrentSortMetric] =
    useOrderByParam('latestFailureTime');
  const [isAscending, setIsAscending] = useState(false);
  const joinedGroups = selectedVariantGroups.join(',');
  const {
    isPending,
    data: failures,
    error,
    isSuccess,
  } = useFetchClusterFailures(
    project,
    clusterAlgorithm,
    clusterId,
    filterToMetric,
  );

  useEffect(() => {
    if (failures) {
      const variantGroups = countDistictVariantValues(failures);
      setVariantGroups(variantGroups);

      // Find the variant groups corresponding to the selection.
      // Note the grouping keys are ordered and this order should
      // be preserved.
      const selectedVGs: VariantGroup[] = [];
      selectedVariantGroups.forEach((key) => {
        const vg = variantGroups.find((vg) => vg.key === key);
        if (vg !== undefined) {
          selectedVGs.push(vg);
        }
      });
      let updatedGroups = groupAndCountFailures(failures || [], selectedVGs);
      updatedGroups = countFailures(updatedGroups, impactFilter);
      setGroups(sortFailureGroups(updatedGroups, sortMetric, isAscending));
    }
    // Use selectedVariantGroups.join(',') instead of selectedVariantGroups
    // as selectedVariantGroups changes every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [failures, sortMetric, isAscending, impactFilter, joinedGroups]);

  const onImpactFilterChanged = (event: SelectChangeEvent) => {
    setImpactFilter(
      ImpactFilters.filter((filter) => filter.id === event.target.value)?.[0] ||
        ImpactFilters[1],
    );
  };

  const onMetricFilterChanged = (event: SelectChangeEvent) => {
    setFilterToMetric(metrics.find((m) => m.metricId === event.target.value));
  };

  const handleVariantsChange = (
    event: SelectChangeEvent<typeof selectedVariantGroups>,
  ) => {
    const value = event.target.value;
    setSelectedVariantGroups(
      typeof value === 'string' ? value.split(',') : value,
      true,
    );
  };

  const toggleSort = (metric: MetricName) => {
    if (metric === sortMetric) {
      setIsAscending(!isAscending);
    } else {
      setCurrentSortMetric(metric);
      setIsAscending(false);
    }
  };

  return (
    <>
      <Typography paragraph color="GrayText">
        Failures are grouped: to expand, click the <ChevronRightIcon /> next to
        the test name or group.
      </Typography>
      <FailuresTableFilter
        metrics={metrics}
        metricFilter={filterToMetric}
        onMetricFilterChanged={onMetricFilterChanged}
        impactFilter={impactFilter}
        onImpactFilterChanged={onImpactFilterChanged}
        variantGroups={variantGroups}
        selectedVariantGroups={selectedVariantGroups}
        handleVariantGroupsChange={handleVariantsChange}
      />
      {error && <LoadErrorAlert entityName="recent failures" error={error} />}
      {isPending && <CentralizedProgress />}
      {isSuccess && failures !== undefined && (
        <Table size="small">
          <FailuresTableHead
            toggleSort={toggleSort}
            sortMetric={sortMetric}
            isAscending={isAscending}
          />
          <TableBody>
            {groups.map((group) => (
              <FailuresTableGroup
                project={project}
                key={group.id}
                group={group}
                selectedVariantGroups={selectedVariantGroups}
              />
            ))}
            {groups.length === 0 && (
              <TableRow>
                <TableCell colSpan={11}>
                  Hooray! There were no failures matching the filter criteria in
                  the last week.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      )}
    </>
  );
};

export default FailuresTab;
