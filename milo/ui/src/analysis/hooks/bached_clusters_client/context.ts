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

import { DecoratedClient } from '@/common/hooks/prpc_query';
import { BatchedClustersClientImpl } from '@/proto_utils/batched_clusters_client';

export const BatchedClustersClientCtx = createContext<
  DecoratedClient<BatchedClustersClientImpl> | undefined
>(undefined);

export interface BatchedClustersClientProviderProps {
  readonly children: ReactNode;
}
export function useBatchedClustersClient() {
  const ctx = useContext(BatchedClustersClientCtx);
  if (ctx === undefined) {
    throw new Error(
      'useBatchedClustersClient can only be used in a BatchedClustersClientProvider',
    );
  }
  return ctx;
}
