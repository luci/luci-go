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

// See luci.analysis.v1.Projects.GetProjectConfigResponse.Monorail for documentation.
export interface Monorail {
  // The monorail project used for this LUCI project.
  project: string;

  // The shortlink format used for this bug tracker.
  // For example, "crbug.com".
  displayPrefix: string;
}

// See luci.analysis.v1.Projects.ProjectConfig for documentation.
export interface ProjectConfig {
  // The format is: `projects/{project}/config`.
  name: string;

  // Details about the monorail project used for this LUCI project.
  monorail: Monorail;
}
