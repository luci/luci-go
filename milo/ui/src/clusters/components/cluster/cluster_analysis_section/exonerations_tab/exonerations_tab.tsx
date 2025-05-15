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

import TabPanel from '@mui/lab/TabPanel';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { useQuery } from '@tanstack/react-query';
import { useContext, useEffect, useState } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import {
  ChromeOSCriteria,
  ChromiumCriteria,
  ExoneratedTestVariant,
  ExonerationCriteria,
  SortableField,
  sortTestVariants,
  testVariantFromAnalysis,
} from '@/clusters/components/cluster/cluster_analysis_section/exonerations_tab/model/model';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import {
  useClustersService,
  useTestVariantsService,
} from '@/clusters/services/services';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';
import {
  QueryTestVariantFailureRateRequest,
  QueryTestVariantFailureRateRequest_TestVariant,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

import { ClusterContext } from '../../cluster_context';

import ExonerationsTableHead from './exonerations_table_head/exonerations_table_head';
import ExonerationsTableRow from './exonerations_table_row/exonerations_table_row';

interface Props {
  // The name of the tab.
  value: string;
}

const ExonerationsTab = ({ value }: Props) => {
  const {
    project,
    algorithm: clusterAlgorithm,
    id: clusterId,
  } = useContext(ClusterContext);

  const [testVariants, setTestVariants] = useState<ExoneratedTestVariant[]>([]);
  const service = useClustersService();
  const tvService = useTestVariantsService();

  const [sortField, setCurrentSortField] =
    useState<SortableField>('lastExoneration');
  const [isAscending, setIsAscending] = useState(false);
  const [criteria] = useState<ExonerationCriteria>(
    project === 'chromeos' ? ChromeOSCriteria : ChromiumCriteria,
  );

  const {
    isPending,
    isSuccess,
    data: unsortedTestVariants,
    error,
  } = useQuery({
    queryKey: ['exoneratedTestVariants', project, clusterAlgorithm, clusterId],

    queryFn: async () => {
      const clusterResponse = await service.QueryExoneratedTestVariants({
        parent: `projects/${project}/clusters/${clusterAlgorithm}/${clusterId}/exoneratedTestVariants`,
      });
      const clusterExoneratedTestVariants = clusterResponse.testVariants;
      if (clusterExoneratedTestVariants.length === 0) {
        return [];
      }
      const tvRequest: QueryTestVariantFailureRateRequest = {
        project: project,
        testVariants: clusterExoneratedTestVariants.map((v) => {
          return QueryTestVariantFailureRateRequest_TestVariant.create({
            testId: v.testId,
            variant: v.variant,
          });
        }),
      };
      const tvResponse = await tvService.QueryFailureRate(tvRequest);
      return (
        tvResponse.testVariants?.map((analyzedTV, i) => {
          // QueryFailureRate returns test variants in the same order
          // that they are requested.
          const exoneratedTV = clusterExoneratedTestVariants[i];
          return testVariantFromAnalysis(exoneratedTV, analyzedTV);
        }) || []
      );
    },

    retry: prpcRetrier,
  });

  useEffect(() => {
    if (unsortedTestVariants) {
      setTestVariants(
        sortTestVariants(
          criteria,
          unsortedTestVariants,
          sortField,
          isAscending,
        ),
      );
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
      {error && (
        <LoadErrorAlert entityName="exonerated test variants" error={error} />
      )}
      {isPending && <CentralizedProgress />}
      {isSuccess && (
        <Table size="small">
          <ExonerationsTableHead
            toggleSort={toggleSort}
            sortField={sortField}
            isAscending={isAscending}
          />
          <TableBody>
            {testVariants.map((tv) => (
              <ExonerationsTableRow
                criteria={criteria}
                project={project}
                testVariant={tv}
                key={tv.key}
              />
            ))}
            {testVariants.length === 0 && (
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

export default ExonerationsTab;
