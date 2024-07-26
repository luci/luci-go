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

import { ClustersClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { MetricsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/metrics.pb';
import { ProjectsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';
import { RulesClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/rules.pb';
import { TestVariantsClientImpl } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

import { PrpcClient } from './prpc_client';
import { getSessionAccessToken } from './auth_token';

declare global {
  interface Window {
    // Host to use for pRPC requests to LUCI Analysis. E.g. analysis.api.luci.app.
    luciAnalysisHostname: string;
  }
}

export const getClustersService = () => {
  return new ClustersClientImpl(getLUCIAnalysisPrpcClient());
};

export const getMetricsService = () => {
  return new MetricsClientImpl(getLUCIAnalysisPrpcClient());
};

export const getProjectsService = () => {
  return new ProjectsClientImpl(getLUCIAnalysisPrpcClient());
};

export const getRulesService = () => {
  return new RulesClientImpl(getLUCIAnalysisPrpcClient());
};

export const getTestVariantsService = () => {
  return new TestVariantsClientImpl(getLUCIAnalysisPrpcClient());
};

function getLUCIAnalysisPrpcClient(): PrpcClient {
  const host = window.luciAnalysisHostname;
  // Only use HTTP with local development servers hosted
  // on the same machine and where the page is loaded over
  // HTTP.
  const isLoopback = (host === 'localhost' || host.startsWith('localhost:') || host === '127.0.0.1' || host.startsWith('127.0.0.1:'));
  const insecure = isLoopback && document.location.protocol === 'http:';
  return new PrpcClient({
    host: host,
    insecure: insecure,
    getAuthToken: getSessionAccessToken,
  });
}
