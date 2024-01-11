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

import TabPanel from '@mui/lab/TabPanel';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';

import CentralizedProgress from '@/components/centralized_progress/centralized_progress';
import {
  ChromeOSCriteria,
  ChromiumCriteria,
  ExoneratedTestVariant,
  ExonerationCriteria,
  SortableField,
  sortTestVariants,
  testVariantFromAnalysis,
} from '@/components/cluster/cluster_analysis_section/exonerations_tab/model/model';
import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import { getClustersService } from '@/legacy_services/cluster';
import { prpcRetrier } from '@/legacy_services/shared_models';
import {
  getTestVariantsService,
  QueryTestVariantFailureRateRequest,
} from '@/legacy_services/test_variants';

import { ClusterContext } from '../../cluster_context';
import ExonerationsTableHead from './exonerations_table_head/exonerations_table_head';
import ExonerationsTableRow from './exonerations_table_row/exonerations_table_row';

interface Props {
  // The name of the tab.
  value: string;
}

const ExonerationsTab = ({
  value,
}: Props) => {
  const {
    project,
    algorithm: clusterAlgorithm,
    id: clusterId,
  } = useContext(ClusterContext);

  const [testVariants, setTestVariants] = useState<ExoneratedTestVariant[]>([]);

  const [sortField, setCurrentSortField] = useState<SortableField>('lastExoneration');
  const [isAscending, setIsAscending] = useState(false);
  const [criteria] = useState<ExonerationCriteria>(project == 'chromeos' ? ChromeOSCriteria : ChromiumCriteria);

  const {
    isLoading,
    isSuccess,
    data: unsortedTestVariants,
    error,
  } = useQuery(
      ['exoneratedTestVariants', project, clusterAlgorithm, clusterId],
      async () => {
        const service = getClustersService();
        const clusterResponse = await service.queryExoneratedTestVariants({
          parent: `projects/${project}/clusters/${clusterAlgorithm}/${clusterId}/exoneratedTestVariants`,
        });
        const clusterExoneratedTestVariants = clusterResponse.testVariants;
        if (!clusterExoneratedTestVariants) {
          return [];
        }
        const tvRequest: QueryTestVariantFailureRateRequest = {
          project: project,
          testVariants: clusterExoneratedTestVariants.map((v) => {
            return {
              testId: v.testId,
              variant: v.variant,
            };
          }),
        };
        const tvService = getTestVariantsService();
        const tvResponse = await tvService.queryFailureRate(tvRequest);
        return tvResponse.testVariants?.map((analyzedTV, i) => {
          // QueryFailureRate returns test variants in the same order
          // that they are requested.
          const exoneratedTV = clusterExoneratedTestVariants[i];
          return testVariantFromAnalysis(exoneratedTV, analyzedTV);
        }) || [];
      }, {
        retry: prpcRetrier,
      });

  useEffect(() => {
    if (unsortedTestVariants) {
      setTestVariants(sortTestVariants(criteria, unsortedTestVariants, sortField, isAscending));
    }
  }, [criteria, unsortedTestVariants, sortField, isAscending]);

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
      {
        error && (
          <LoadErrorAlert
            entityName='exonerated test variants'
            error={error}
          />
        )
      }
      {
        isLoading && (
          <CentralizedProgress />
        )
      }
      {
        isSuccess && (
          <Table size="small">
            <ExonerationsTableHead
              toggleSort={toggleSort}
              sortField={sortField}
              isAscending={isAscending}/>
            <TableBody>
              {
                testVariants.map((tv) => (
                  <ExonerationsTableRow
                    criteria={criteria}
                    project={project}
                    testVariant={tv}
                    key={tv.key}/>
                ))
              }
              {
                testVariants.length == 0 && (
                  <TableRow>
                    <TableCell colSpan={6}>Hooray! There were no presubmit-blocking failures exonerated in the last week.</TableCell>
                  </TableRow>
                )
              }
            </TableBody>
          </Table>
        )
      }
    </TabPanel>
  );
};

export default ExonerationsTab;
