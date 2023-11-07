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

// An enum that represents the bug filing system that the project uses.
export type BugSystem =
  | 'BUG_SYSTEM_UNSPECIFIED'
  | 'MONORAIL'
  | 'BUGANIZER';

export type BuganizerPriority =
  | 'BUGANIZER_PRIORITY_UNSPECIFIED'
  | 'P0'
  | 'P1'
  | 'P2'
  | 'P3'
  | 'P4';

export const getProjectsService = () => {
  const client = new AuthorizedPrpcClient();
  return new ProjectService(client);
};

// A service to handle projects related gRPC requests.
export class ProjectService {
  private static SERVICE = 'luci.analysis.v1.Projects';

  client: AuthorizedPrpcClient;

  constructor(client: AuthorizedPrpcClient) {
    this.client = client;
  }

  async getConfig(request: GetProjectConfigRequest): Promise<ProjectConfig> {
    return this.client.call(ProjectService.SERVICE, 'GetConfig', request);
  }

  async list(request: ListProjectsRequest): Promise<ListProjectsResponse> {
    return this.client.call(ProjectService.SERVICE, 'List', request);
  }
}

// eslint-disable-next-line @typescript-eslint/no-empty-interface
export interface ListProjectsRequest {}

export interface Project {
    // The format is: `projects/{project}`.
    name: string;
    displayName: string;
    project: string,
}

export interface ListProjectsResponse {
    projects?: Project[];
}

export interface GetProjectConfigRequest {
  // The format is: `projects/{project}/config`.
  name: string;
}

// MetricThreshold specifies thresholds for a particular metric.
// The threshold is considered satisfied if any of the individual metric
// thresholds is met or exceeded (i.e. if multiple thresholds are set, they
// are combined using an OR-semantic). If no threshold is set, the threshold
// as a whole is unsatisfiable.
export interface MetricThreshold {
  // The threshold for one day.
  oneDay?: string;

  // The threshold for three days.
  threeDay?: string;

  // The threshold for seven days.
  sevenDay?: string;
}

export interface ImpactMetricThreshold {
  metricId: string
  threshold: MetricThreshold;
}

export interface BuganizerProjectPriorityMapping {
  // The priority value.
  priority: BuganizerPriority;

  // The thresholds at which to apply the priority.
  // The thresholds are considered satisfied if any of the individual impact
  // metric thresholds is met or exceeded (i.e. if multiple thresholds are set,
  // they are combined using an OR-semantic).
  thresholds: ImpactMetricThreshold[];
}

export interface BuganizerProject {
  priorityMappings?: BuganizerProjectPriorityMapping[];
}

export interface MonorailPriority {
  // The priority value.
  priority: string;

  // The thresholds at which to apply the priority.
  // The thresholds are considered satisfied if any of the individual impact
  // metric thresholds is met or exceeded (i.e. if multiple thresholds are set,
  // they are combined using an OR-semantic).
  thresholds: ImpactMetricThreshold[];
}

// See luci.analysis.v1.Projects.GetProjectConfigResponse.Monorail for documentation.
// Captures the fields set when populated inside the BugManagement message.
export interface MonorailProject {
  // The monorail project used for this LUCI project.
  project: string;

  // The shortlink format used for this bug tracker.
  // For example, "crbug.com".
  displayPrefix: string;
}

export interface BugManagementPolicyMetric {
  metricId: string;
  activationThreshold: MetricThreshold;
  deactivationThreshold: MetricThreshold;
}

export interface BugManagementPolicyExplanation {
  // Must be sanitized by UI before rendering.
  problemHtml: string;
  // Must be sanitized by UI before rendering.
  actionHtml: string;
}

export interface BugManagementPolicy {
  id: string;
  owners: string[];
  humanReadableName: string;
  priority: BuganizerPriority;
  metrics: BugManagementPolicyMetric[];
  explanation: BugManagementPolicyExplanation;
}

export interface BugManagement {
  // Details about the monorail project used for this LUCI project.
  monorail?: MonorailProject;
  policies?: BugManagementPolicy[];
}

// See luci.analysis.v1.Projects.ProjectConfig for documentation.
export interface ProjectConfig {
  // The format is: `projects/{project}/config`.
  name: string;

  // Configuration for automatic bug management.
  bugManagement: BugManagement;
}
