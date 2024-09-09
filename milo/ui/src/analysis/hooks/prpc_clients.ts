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

import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { ChangepointsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { TestHistoryClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';
import { TestVariantBranchesClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { BatchedClustersClientImpl } from '@/proto_utils/batched_clusters_client';

export function useChangepointsClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: ChangepointsClientImpl,
  });
}

export function useTestVariantBranchesClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: TestVariantBranchesClientImpl,
  });
}

export function useTestHistoryClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: TestHistoryClientImpl,
  });
}

export function useBatchedClustersClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: BatchedClustersClientImpl,
  });
}
