// Copyright 2022 The LUCI Authors.
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

import { AuthorizedPrpcClient } from '@/clients/authorized_client';

import {
  MetricId,
} from './shared_models';

export const getMetricsService = () => {
  const client = new AuthorizedPrpcClient();
  return new MetricsService(client);
};

// A service to enumerate metrics.
export class MetricsService {
  private static SERVICE = 'luci.analysis.v1.Metrics';

  client: AuthorizedPrpcClient;

  constructor(client: AuthorizedPrpcClient) {
    this.client = client;
  }

  async listForProject(request: ListProjectMetricsRequest): Promise<ListProjectMetricsResponse> {
    return this.client.call(MetricsService.SERVICE, 'ListForProject', request);
  }
}

// eslint-disable-next-line @typescript-eslint/no-empty-interface
export interface ListProjectMetricsRequest {
  // The parent LUCI Project, which owns the collection of metrics.
  // Format: projects/{project}.
  parent: string;
}

export interface ListProjectMetricsResponse {
    metrics?: Metric[];
}

export interface Metric {
    // The resource name of the metric.
    // Format: projects/{project}/metrics/{metric_id}.
    name: string;
    // The identifier of the metric.
    metricId: MetricId;
    // The human readable name of the metric.
    humanReadableName: string;
    // The human readable description of the metric. This typically appears
    // in a help popup next to the metric name.
    description: string;
    // Whether the metric should be shown by default in
    // the cluster listing and on cluster pages.
    isDefault?: boolean;
    // Defines the default sort order of metrics.
    // The metric with the highest sort priority defines
    // the primary sort order, followed by the metric
    // with the second highest sort priority, and so on.
    sortPriority: number;
}
