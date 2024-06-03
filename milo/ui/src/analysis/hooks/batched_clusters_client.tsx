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

import { ReactNode, createContext, useContext } from 'react';

import {
  DecoratedClient,
  usePrpcServiceClient,
} from '@/common/hooks/prpc_query';
import { BatchedClustersClientImpl } from '@/proto_utils/batched_clusters_client';

const BatchedClustersClientCtx = createContext<
  DecoratedClient<BatchedClustersClientImpl> | undefined
>(undefined);

export interface BatchedClustersClientProviderProps {
  readonly children: ReactNode;
}

export function BatchedClustersClientProvider({
  children,
}: BatchedClustersClientProviderProps) {
  // Use a single client instance so all requests in the same rendering cycle
  // can be batched together.
  const client = usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: BatchedClustersClientImpl,
  });

  return (
    <BatchedClustersClientCtx.Provider value={client}>
      {children}
    </BatchedClustersClientCtx.Provider>
  );
}

export function useBatchedClustersClient() {
  const ctx = useContext(BatchedClustersClientCtx);
  if (ctx === undefined) {
    throw new Error(
      'useBatchedClustersClient must be used in a BatchedClustersClientProvider',
    );
  }
  return ctx;
}
