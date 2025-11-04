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

import { ChangepointsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';
import { TestHistoryClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';
import { TestVariantBranchesClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';
import { TestVariantsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';
import { AnalysesClientImpl } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { BuildsClientImpl } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { MiloInternalClientImpl } from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';
import { ResultDBClientImpl } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { TreesClientImpl } from '@/proto/go.chromium.org/luci/tree_status/proto/v1/trees.pb';
import { BatchedClustersClientImpl } from '@/proto_utils/batched_clients/clusters_client';
import { BatchedMiloInternalClientImpl } from '@/proto_utils/batched_clients/milo_internal_client';

import { usePrpcServiceClient } from './prpc_query';

export function useMiloInternalClient() {
  return usePrpcServiceClient({
    host: SETTINGS.milo.host,
    ClientImpl: MiloInternalClientImpl,
  });
}

export function useBatchedMiloInternalClient() {
  return usePrpcServiceClient({
    host: SETTINGS.milo.host,
    ClientImpl: BatchedMiloInternalClientImpl,
  });
}

export function useTreesClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciTreeStatus.host,
    ClientImpl: TreesClientImpl,
  });
}

export function useResultDbClient() {
  return usePrpcServiceClient({
    host: SETTINGS.resultdb.host,
    ClientImpl: ResultDBClientImpl,
  });
}

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

export function useTestVariantsClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: TestVariantsClientImpl,
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

export function useAnalysesClient() {
  return usePrpcServiceClient({
    host: SETTINGS.luciBisection.host,
    ClientImpl: AnalysesClientImpl,
  });
}

export function useBuildsClient() {
  return usePrpcServiceClient({
    host: SETTINGS.buildbucket.host,
    ClientImpl: BuildsClientImpl,
  });
}
