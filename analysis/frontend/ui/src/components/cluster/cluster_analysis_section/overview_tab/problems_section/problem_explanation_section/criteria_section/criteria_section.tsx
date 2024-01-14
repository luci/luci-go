// Copyright 2023 The LUCI Authors.
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

import { useContext } from 'react';

import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';

import LoadErrorAlert from '@/components/load_error_alert/load_error_alert';
import CentralizedProgress from '@/components/centralized_progress/centralized_progress';
import { ClusterContext } from '@/components/cluster/cluster_context';
import useFetchCluster from '@/hooks/use_fetch_cluster';
import { BugManagementPolicy } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';

import { OverviewTabContextData } from '../../../overview_tab_context';
import { criteriaForPolicy, Criterium } from './criteria';

const criteriumLabel = (criterium: Criterium) => {
  return (
    <>
      <Tooltip arrow title={criterium.metricDescription}>
        <Typography component='span' sx={{
          textDecoration: 'underline',
          textDecorationStyle: 'dotted',
          textUnderlinePosition: 'under',
          fontSize: 'inherit',
        }}>
          {criterium.metricName}
        </Typography>
      </Tooltip>
      &nbsp;({criterium.durationKey}) (value: {criterium.currentValue}) {criterium.greaterThanOrEqual ? '\u2265' : '<'} {criterium.thresholdValue}
    </>
  );
};

export interface Props {
  policy: BugManagementPolicy;
  showActivationCriteria: boolean; // whether we are to show the activation or deactivation criteria.
}

export const CriteriaSection = ({ policy, showActivationCriteria }: Props) => {
  const { metrics } = useContext(OverviewTabContextData);
  const clusterId = useContext(ClusterContext);
  const {
    isLoading: isLoading,
    error: error,
    data: cluster,
  } = useFetchCluster(clusterId.project, clusterId.algorithm, clusterId.id);

  const criteria = criteriaForPolicy(policy, metrics, cluster?.metrics, showActivationCriteria);

  if (error) {
    return <LoadErrorAlert entityName="cluster" error={error} />;
  }
  if (isLoading) {
    return <CentralizedProgress />;
  }

  return (
    <>
      {
        cluster && (
          criteria.map((criterium, i) => {
            return <Box
              key={`${criterium.metricId}:${criterium.durationKey}`} sx={{ paddingBottom: '8px' }}>
              <Chip
                variant="outlined"
                color={criterium.satisfied ? 'success' : 'default'}
                label={criteriumLabel(criterium)} />
              {(i < criteria.length - 1) &&
                // A policy activates if ANY of the possible metric thresholds is met (i.e. OR of criteria),
                // whereas deactivation only occurs if ALL possible metric thresholds are no longer met
                // (i.e. an AND of criteria).
                <Typography
                  component="span"
                  sx={{ paddingLeft: '0.5rem' }} >
                  { showActivationCriteria ? 'OR' : 'AND' }
                </Typography>
              }
            </Box>;
          })
        )
      }
    </>
  );
};
