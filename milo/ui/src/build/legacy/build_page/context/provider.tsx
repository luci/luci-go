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

import { Alert } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { ReactNode } from 'react';
import { useLocation } from 'react-router';

import { isFailureStatus } from '@/build/tools/build_utils';
import { OutputBuild } from '@/build/types';
import { useAnalysesClient } from '@/common/hooks/prpc_clients';
import { QueryAnalysisRequest } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { BuildCtx } from './context';
export interface BuildProviderProps {
  readonly build?: OutputBuild;
  readonly children: ReactNode;
}

export function BuildContextProvider({ build, children }: BuildProviderProps) {
  const location = useLocation();
  const isBlamelistTab = location.pathname.endsWith('/blamelist');
  const isTryJob = (build?.input?.gerritChanges?.length ?? 0) > 0;
  const analysisClient = useAnalysesClient();
  const {
    data: response,
    isError,
    error,
  } = useQuery({
    ...analysisClient.QueryAnalysis.query(
      QueryAnalysisRequest.fromPartial({
        buildFailure: {
          bbid: build?.id,
          failedStepName: 'compile',
        },
      }),
    ),
    // only use the query if:
    // 1. a Buildbucket ID has been provided
    // 2. the build has failed (bisection only runs on failed builds; querying
    //    passing builds by BBID will never return results)
    // 3. it is not a try job (bisection is only for post-submit)
    // 4. we are on the blamelist tab
    enabled: !!(
      build &&
      build.id &&
      isFailureStatus(build.status) &&
      !isTryJob &&
      isBlamelistTab
    ),
    throwOnError: false,
    retry: false,
  });
  const analysis = response?.analyses[0];
  return (
    <BuildCtx.Provider value={{ build, analysis, analysisError: error }}>
      {isError && (
        <Alert severity="warning" sx={{ margin: '10px' }}>
          Failed to load secondary data. If you log in you may be able to see
          more information on this page.
        </Alert>
      )}
      {children}
    </BuildCtx.Provider>
  );
}
