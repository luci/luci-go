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

declare global {
    interface Window { monorailHostname: string | undefined; }
}

export function getIssuesService() : IssuesService {
  const useIDToken = true;
  if (!window.monorailHostname) {
    throw new Error('monorail hostname not set');
  }
  const client = new AuthorizedPrpcClient('api-dot-' + window.monorailHostname, useIDToken);
  return new IssuesService(client);
}

/**
 * Provides access to the monorail issues service over pRPC.
 * For handling errors, import:
 * import { GrpcError } from '@chopsui/prpc-client';
 */
export class IssuesService {
  private static SERVICE = 'monorail.v3.Issues';

  client: AuthorizedPrpcClient;

  constructor(client: AuthorizedPrpcClient) {
    this.client = client;
  }

  async getIssue(request: GetIssueRequest) : Promise<Issue> {
    return this.client.call(IssuesService.SERVICE, 'GetIssue', request, {});
  }
}

export interface GetIssueRequest {
    // The name of the issue to request.
    // Format: projects/{project}/issues/{issue_id}.
    name: string;
}

export interface StatusValue {
    status: string;
    derivation: string;
}

// Definition here is partial. Full definition here:
// https://source.chromium.org/chromium/infra/infra/+/main:appengine/monorail/api/v3/api_proto/issue_objects.proto
export interface Issue {
    name: string;
    summary: string;
    status: StatusValue;
    reporter: string;
    modifyTime: string; // RFC 3339 encoded date/time.
}
