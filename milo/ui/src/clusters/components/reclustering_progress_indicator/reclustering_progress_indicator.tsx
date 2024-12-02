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

import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Grid from '@mui/material/Grid2';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';

import CircularProgressWithLabel from '@/clusters/components/circular_progress_with_label/circular_progress_with_label';
import LoadErrorAlert from '@/clusters/components/load_error_alert/load_error_alert';
import { useClustersService } from '@/clusters/services/services';
import {
  fetchProgress,
  noProgressToShow,
  progressNotYetStarted,
  progressToLatestAlgorithms,
  progressToLatestConfig,
  progressToRulesVersion,
} from '@/clusters/tools/progress_tools';
import { prpcRetrier } from '@/clusters/tools/prpc_retrier';

interface Props {
  project: string;
  hasRule?: boolean | undefined;
  rulePredicateLastUpdated?: string | undefined;
}

const ReclusteringProgressIndicator = ({
  project,
  hasRule,
  rulePredicateLastUpdated,
}: Props) => {
  const [show, setShow] = useState(false);

  const [progressPerMille, setProgressPerMille] = useState(noProgressToShow);
  const [reclusteringTarget, setReclusteringTarget] = useState('');
  const queryClient = useQueryClient();
  const clustersService = useClustersService();

  const {
    isLoading,
    data: progress,
    error,
  } = useQuery(
    ['reclusteringProgress', project],
    async () => {
      return await fetchProgress(project, clustersService);
    },
    {
      refetchInterval: () => {
        // Only update the progress if we are still less than 100%
        if (progressPerMille >= 1000) {
          return false;
        }
        return 1000;
      },
      retry: prpcRetrier,
    },
  );

  useEffect(() => {
    if (progress) {
      let currentProgressPerMille = progressToLatestAlgorithms(progress);
      let currentTarget = 'updated clustering algorithms';
      const configProgress = progressToLatestConfig(progress);
      if (configProgress < currentProgressPerMille) {
        currentTarget = 'updated clustering configuration';
        currentProgressPerMille = configProgress;
      }
      if (hasRule && rulePredicateLastUpdated) {
        const ruleProgress = progressToRulesVersion(
          progress,
          rulePredicateLastUpdated,
        );
        if (ruleProgress < currentProgressPerMille) {
          currentTarget = 'the latest rule definition';
          currentProgressPerMille = ruleProgress;
        }
      }

      setReclusteringTarget(currentTarget);
      setProgressPerMille(currentProgressPerMille);
    }
  }, [progress, rulePredicateLastUpdated, hasRule]);

  useEffect(() => {
    if (progressPerMille >= progressNotYetStarted && progressPerMille < 1000) {
      setShow(true);
    }
  }, [progressPerMille]);

  if (error) {
    return <LoadErrorAlert entityName="reclustering progress" error={error} />;
  }

  if (isLoading && !progress) {
    // no need to show anything if there is no progress and we are still loading
    return <></>;
  }

  const handleRefreshAnalysis = () => {
    queryClient.invalidateQueries(['cluster']);
    queryClient.invalidateQueries(['clusterFailures']);
    setShow(false);
  };

  let progressText = 'task queued';
  if (progressPerMille >= 0) {
    progressText = (progressPerMille / 10).toFixed(1) + '%';
  }

  const progressContent = () => {
    if (progressPerMille < 1000) {
      return (
        <>
          <p>
            LUCI Analysis is re-clustering test results to reflect{' '}
            {reclusteringTarget} ({progressText}). Cluster impact may be
            out-of-date.
          </p>
        </>
      );
    } else {
      return 'LUCI Analysis has finished re-clustering test results. Updated cluster impact is now available.';
    }
  };
  return (
    <>
      {show && (
        <Alert
          severity={progressPerMille >= 1000 ? 'success' : 'info'}
          icon={false}
          sx={{
            mt: 1,
          }}
        >
          <Grid
            container
            justifyContent="center"
            alignItems="center"
            columnSpacing={{ xs: 2 }}
          >
            <Grid>
              <CircularProgressWithLabel
                variant="determinate"
                value={Math.max(0, progressPerMille / 10)}
              />
            </Grid>
            <Grid data-testid="reclustering-progress-description">
              {progressContent()}
            </Grid>
            <Grid>
              {progressPerMille >= 1000 && (
                <Button
                  color="inherit"
                  size="small"
                  onClick={handleRefreshAnalysis}
                >
                  View updated impact
                </Button>
              )}
            </Grid>
          </Grid>
        </Alert>
      )}
    </>
  );
};

export default ReclusteringProgressIndicator;
