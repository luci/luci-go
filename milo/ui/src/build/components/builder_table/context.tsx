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

import { createContext, ReactNode } from 'react';

import {
  DecoratedClient,
  usePrpcServiceClient,
} from '@/common/hooks/prpc_query';
import { BatchedBuildsClientImpl } from '@/proto_utils/batched_builds_client';

export const BuildsClientCtx =
  createContext<DecoratedClient<BatchedBuildsClientImpl> | null>(null);
export const NumOfBuildsCtx = createContext<number | null>(null);

export interface BuilderTableContextProviderProps {
  readonly numOfBuilds: number;
  readonly maxBatchSize: number;
  readonly children: ReactNode;
}

export function BuilderTableContextProvider({
  numOfBuilds,
  maxBatchSize,
  children,
}: BuilderTableContextProviderProps) {
  // Use a single client instance so all requests in the same rendering cycle
  // can be batched together.
  const buildsClient = usePrpcServiceClient(
    {
      host: SETTINGS.buildbucket.host,
      ClientImpl: BatchedBuildsClientImpl,
    },
    { maxBatchSize },
  );

  return (
    <BuildsClientCtx.Provider value={buildsClient}>
      <NumOfBuildsCtx.Provider value={numOfBuilds}>
        {children}
      </NumOfBuildsCtx.Provider>
    </BuildsClientCtx.Provider>
  );
}
