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

import { DateTime } from 'luxon';

import { DecoratedClient } from '@/common/hooks/prpc_query';
import {
  ClusteringVersion,
  ClustersClientImpl,
  GetReclusteringProgressRequest,
  ReclusteringProgress,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

export const fetchProgress = async (
  project: string,
  clustersService: DecoratedClient<ClustersClientImpl>,
): Promise<ReclusteringProgress> => {
  const request: GetReclusteringProgressRequest = {
    name: `projects/${encodeURIComponent(project)}/reclusteringProgress`,
  };
  const response = await clustersService.GetReclusteringProgress(request);
  return response;
};

export const progressNotYetStarted = -1;
export const noProgressToShow = -2;

export const progressToLatestAlgorithms = (
  progress: ReclusteringProgress,
): number => {
  return progressTo(progress, (target: ClusteringVersion) => {
    // 'next' will be set on all progress objects.
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    return target.algorithmsVersion >= progress.next!.algorithmsVersion;
  });
};

export const progressToLatestConfig = (
  progress: ReclusteringProgress,
): number => {
  // 'next' will be set on all progress objects.
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  const targetConfigVersion = DateTime.fromISO(
    progress.next!.configVersion || '',
  );
  return progressTo(progress, (target: ClusteringVersion) => {
    return DateTime.fromISO(target.configVersion || '') >= targetConfigVersion;
  });
};

export const progressToRulesVersion = (
  progress: ReclusteringProgress,
  rulesVersion: string,
): number => {
  const ruleDate = DateTime.fromISO(rulesVersion);
  return progressTo(progress, (target: ClusteringVersion) => {
    return DateTime.fromISO(target.rulesVersion || '') >= ruleDate;
  });
};

// progressTo returns the progress to completing a re-clustering run
// satisfying the given re-clustering target, expressed as a predicate.
// If re-clustering to a goal that would satisfy the target has started,
// the returned value is value from 0 to 1000. If the run is pending,
// the value -1 is returned.
const progressTo = (
  progress: ReclusteringProgress,
  predicate: (target: ClusteringVersion) => boolean,
): number => {
  // 'last' will be set on all progress objects.
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  if (predicate(progress.last!)) {
    // Completed
    return 1000;
  }

  // 'next' will be set on all progress objects.
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  if (predicate(progress.next!)) {
    return progress.progressPerMille || 0;
  }
  // Run not yet started (e.g. because we are still finishing a previous
  // re-clustering).
  return progressNotYetStarted;
};
