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
import { ClustersClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { MetricsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { ProjectsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';
import { RulesClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';
import { TestVariantsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

export const useClustersService = () => {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: ClustersClientImpl,
  });
};

export const useMetricsService = () => {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: MetricsClientImpl,
  });
};

export const useProjectsService = () => {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: ProjectsClientImpl,
  });
};

export const useRulesService = () => {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: RulesClientImpl,
  });
};

export const useTestVariantsService = () => {
  return usePrpcServiceClient({
    host: SETTINGS.luciAnalysis.host,
    ClientImpl: TestVariantsClientImpl,
  });
};
