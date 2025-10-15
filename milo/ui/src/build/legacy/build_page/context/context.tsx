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

import { useQuery } from '@tanstack/react-query';
import { ReactNode, createContext } from 'react';

import { OutputBuild } from '@/build/types';
import { useAnalysesClient } from '@/common/hooks/prpc_clients';
import {
  Analysis,
  QueryAnalysisRequest,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

interface BuildContext {
  readonly build?: OutputBuild;
  readonly analysis?: Analysis;
}
/**
 * `null` means there's no build.
 * `undefined` means the context provider is missing.
 */
export const BuildCtx = createContext<BuildContext | null | undefined>(
  undefined,
);

export interface BuildProviderProps {
  readonly build?: OutputBuild;
  readonly children: ReactNode;
}

export function BuildContextProvider({ build, children }: BuildProviderProps) {
  const analysisClient = useAnalysesClient();
  const { data: response } = useQuery({
    ...analysisClient.QueryAnalysis.query(
      QueryAnalysisRequest.fromPartial({
        buildFailure: {
          bbid: build?.id,
          failedStepName: 'compile',
        },
      }),
    ),
    // only use the query if a Buildbucket ID has been provided
    enabled: !!(build && build.id),
  });
  const analysis = response?.analyses[0];
  return (
    <BuildCtx.Provider value={{ build, analysis }}>
      {' '}
      {children}
    </BuildCtx.Provider>
  );
}
