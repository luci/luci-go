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

import {
  useContext,
  useEffect,
  useState,
} from 'react';
import { useQuery } from 'react-query';
import { useUpdateEffect } from 'react-use';

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import TabPanel from '@mui/lab/TabPanel';
import { SelectChangeEvent } from '@mui/material/Select';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';

import CentralizedProgress from '@/components/centralized_progress/centralized_progress';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import { getClustersService } from '@/services/cluster';
import { prpcRetrier } from '@/services/shared_models';
import {
  countAndSortFailures,
  countDistictVariantValues,
  defaultFailureFilter,
  defaultImpactFilter,
  FailureFilter,
  FailureFilters,
  FailureGroup,
  groupAndCountFailures,
  ImpactFilter,
  ImpactFilters,
  MetricName,
  sortFailureGroups,
  VariantGroup,
} from '@/tools/failures_tools';

import { ClusterContext } from '../../cluster_context';
import FailuresTableFilter from './failures_table_filter/failures_table_filter';
import FailuresTableGroup from './failures_table_group/failures_table_group';
import FailuresTableHead from './failures_table_head/failures_table_head';
import { useSelectedVariantGroupsParam } from './hooks';

interface Props {
  // The name of the tab.
  value: string;
}

const FailuresTable = ({
  value,
}: Props) => {
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

  const [failureFilter, setFailureFilter] = useState<FailureFilter>(defaultFailureFilter);
  const [impactFilter, setImpactFilter] = useState<ImpactFilter>(defaultImpactFilter);
  // TODO: This should be read from the URL instead.
  const [selectedVariantGroups, setSelectedVariantGroups] = useSelectedVariantGroupsParam(variantGroups.map((g) => g.key));

  const [sortMetric, setCurrentSortMetric] = useState<MetricName>('latestFailureTime');
  const [isAscending, setIsAscending] = useState(false);

  const {
    isLoading,
    data: failures,
    error,
    isSuccess,
  } = useQuery(
      ['clusterFailures', project, clusterAlgorithm, clusterId],
      async () => {
        const service = getClustersService();
        const response = await service.queryClusterFailures({
          parent: `projects/${project}/clusters/${clusterAlgorithm}/${clusterId}/failures`,
        });
        return response.failures || [];
      }, {
        retry: prpcRetrier,
      });

  useEffect( () => {
    if (failures) {
      const groups = countDistictVariantValues(failures);
      setVariantGroups(groups);
    }
  }, [failures]);

  useUpdateEffect(() => {
    setGroups(sortFailureGroups(groups, sortMetric, isAscending));
  }, [sortMetric, isAscending]);

  useUpdateEffect(() => {
    setGroups(countAndSortFailures(groups, impactFilter));
  }, [impactFilter]);

  useUpdateEffect(() => {
    groupCountAndSortFailures();
  }, [failureFilter]);

  useUpdateEffect(() => {
    groupCountAndSortFailures();
  }, [variantGroups]);

  useUpdateEffect(() => {
    const variantGroupsClone = [...variantGroups];
    variantGroupsClone.forEach((variantGroup) => {
      variantGroup.isSelected = selectedVariantGroups.includes(variantGroup.key);
    });
    setVariantGroups(variantGroupsClone);
  },
  // We use the length rather than the array for the dependency here, as the
  // object changes on every render leading this function to be executed every
  // render.  Since the UI only allows adding or removing selections and not
  // replacing them, the length will detect all modifications.
  [selectedVariantGroups.length]);

  const groupCountAndSortFailures = () => {
    if (failures) {
      let updatedGroups = groupAndCountFailures(failures, variantGroups, failureFilter);
      updatedGroups = countAndSortFailures(updatedGroups, impactFilter);
      setGroups(sortFailureGroups(updatedGroups, sortMetric, isAscending));
    }
  };

  const onImpactFilterChanged = (event: SelectChangeEvent) => {
    setImpactFilter(ImpactFilters.filter((filter) => filter.name === event.target.value)?.[0] || ImpactFilters[1]);
  };

  const onFailureFilterChanged = (event: SelectChangeEvent) => {
    setFailureFilter((event.target.value as FailureFilter) || FailureFilters[0]);
  };

  const handleVariantsChange = (event: SelectChangeEvent<typeof selectedVariantGroups>) => {
    const value = event.target.value;
    setSelectedVariantGroups(typeof value === 'string' ? value.split(',') : value, true);
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
    <TabPanel value={value}>
      {
        error && (
          <LoadErrorAlert
            entityName="recent failures"
            error={error}
          />
        )
      }
      {
        (isLoading) && (
          <CentralizedProgress />
        )
      }
      {
        (isSuccess && failures !== undefined) && (
          <>
            <Typography paragraph color='GrayText'>To view failures, click the <ChevronRightIcon /> next to the test name or group.</Typography>
            <FailuresTableFilter
              failureFilter={failureFilter}
              onFailureFilterChanged={onFailureFilterChanged}
              impactFilter={impactFilter}
              onImpactFilterChanged={onImpactFilterChanged}
              variantGroups={variantGroups}
              selectedVariantGroups={selectedVariantGroups}
              handleVariantGroupsChange={handleVariantsChange}/>
            <Table size="small">
              <FailuresTableHead
                toggleSort={toggleSort}
                sortMetric={sortMetric}
                isAscending={isAscending}/>
              <TableBody>
                {
                  groups.map((group) => (
                    <FailuresTableGroup
                      project={project}
                      parentKeys={[]}
                      key={group.id}
                      group={group}
                      variantGroups={variantGroups}/>
                  ))
                }
                {
                  groups.length == 0 && (
                    <TableRow>
                      <TableCell colSpan={10}>Hooray! There were no failures matching the filter criteria in the last week.</TableCell>
                    </TableRow>
                  )
                }
              </TableBody>
            </Table>
          </>
        )
      }
    </TabPanel>
  );
};

export default FailuresTable;
