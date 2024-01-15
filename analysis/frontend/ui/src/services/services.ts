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

export const getClustersService = () => {
  const insecure = document.location.protocol === 'http:';
  const client = new ClustersClientImpl(
      new PrpcClient({
        insecure: insecure,
        getAuthToken: getSessionAccessToken,
      }),
  );
  return client;
};

export const getMetricsService = () => {
  const insecure = document.location.protocol === 'http:';
  const client = new MetricsClientImpl(
      new PrpcClient({
        insecure: insecure,
        getAuthToken: getSessionAccessToken,
      }),
  );
  return client;
};

export const getProjectsService = () => {
  const insecure = document.location.protocol === 'http:';
  const client = new ProjectsClientImpl(
      new PrpcClient({
        insecure: insecure,
        getAuthToken: getSessionAccessToken,
      }),
  );
  return client;
};

export const getRulesService = () => {
  const insecure = document.location.protocol === 'http:';
  const client = new RulesClientImpl(
      new PrpcClient({
        insecure: insecure,
        getAuthToken: getSessionAccessToken,
      }),
  );
  return client;
};

export const getTestVariantsService = () => {
  const insecure = document.location.protocol === 'http:';
  const client = new TestVariantsClientImpl(
      new PrpcClient({
        insecure: insecure,
        getAuthToken: getSessionAccessToken,
      }),
  );
  return client;
};
