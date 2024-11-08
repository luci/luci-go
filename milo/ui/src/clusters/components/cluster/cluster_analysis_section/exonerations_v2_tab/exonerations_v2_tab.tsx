// Copyright 2024 The LUCI Authors.
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

import TabPanel from '@mui/lab/TabPanel';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { useContext, useEffect, useState } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import {
  SortableField,
  sortTestVariantBranches,
} from '@/clusters/components/cluster/cluster_analysis_section/exonerations_v2_tab/model/model';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import useFetchExoneratedTestVariantBranches, {
  ExoneratedTestVariantBranch,
} from '@/clusters/hooks/use_fetch_exonerated_test_variant_branches';

import { ClusterContext } from '../../cluster_context';

import ExonerationsTableHead from './exonerations_table_head/exonerations_table_head';
import ExonerationsTableRow from './exonerations_table_row/exonerations_table_row';

interface Props {
  // The name of the tab.
  value: string;
}

const ExonerationsV2Tab = ({ value }: Props) => {
  const {
    project,
    algorithm: clusterAlgorithm,
    id: clusterId,
  } = useContext(ClusterContext);

  const [testVariantBranches, setTestVariants] = useState<
    ExoneratedTestVariantBranch[]
  >([]);

  const [sortField, setCurrentSortField] =
    useState<SortableField>('lastExoneration');
  const [isAscending, setIsAscending] = useState(false);

  const {
    isLoading,
    isSuccess,
    data: response,
    error,
  } = useFetchExoneratedTestVariantBranches(
    project,
    clusterAlgorithm,
    clusterId,
  );

  useEffect(() => {
    if (response?.testVariantBranches !== undefined) {
      setTestVariants(
        sortTestVariantBranches(response, sortField, isAscending),
      );
    }
  }, [response, sortField, isAscending]);

  const toggleSort = (field: SortableField) => {
    if (field === sortField) {
      setIsAscending(!isAscending);
    } else {
      setCurrentSortField(field);
      setIsAscending(false);
    }
  };

  return (
    <TabPanel value={value}>
      {error && (
        <LoadErrorAlert entityName="exonerated test variants" error={error} />
      )}
      {isLoading && <CentralizedProgress />}
      {isSuccess && (
        <Table size="small">
          <ExonerationsTableHead
            toggleSort={toggleSort}
            sortField={sortField}
            isAscending={isAscending}
          />
          <TableBody>
            {testVariantBranches.map((tvb, i) => (
              <ExonerationsTableRow
                criteria={response.criteria}
                project={project}
                testVariantBranch={tvb}
                key={i.toString()}
              />
            ))}
            {testVariantBranches.length === 0 && (
              <TableRow>
                <TableCell colSpan={6}>
                  Hooray! There were no presubmit-blocking failures exonerated
                  in the last week.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      )}
    </TabPanel>
  );
};

export default ExonerationsV2Tab;
