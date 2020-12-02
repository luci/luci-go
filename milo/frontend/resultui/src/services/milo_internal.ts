// Copyright 2020 The LUCI Authors.
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

import { PrpcClient } from '@chopsui/prpc-client';

import { BuilderID, GitilesCommit } from './buildbucket';

/**
 * Manually coded type definition and classes for milo internal service.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source: https://chromium.googlesource.com/infra/luci/luci-go/+/HEAD/milo/api/service/v1/rpc.proto
 */

export interface QueryBlamelistRequest {
  readonly gitilesCommit: GitilesCommit;
  readonly builder: BuilderID;
  readonly multiProjectSupport: boolean;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface CommitUser {
  readonly name: string;
  readonly email: string;
  readonly time: string;
}

export const enum CommitTreeDiffChangeType {
  Add = 'ADD',
  Copy = 'COPY',
  Delete = 'DELETE',
  Modify = 'MODIFY',
  Rename = 'RENAME',
}

export interface CommitTreeDiff {
  readonly type: CommitTreeDiffChangeType;
  readonly oldId: string;
  readonly oldMode: number;
  readonly oldPath: string;
  readonly newId: string;
  readonly newMode: number;
  readonly newPath: string;
}

export interface GitCommit {
  readonly id: string;
  readonly tree: string;
  readonly parents?: string[];
  readonly author: CommitUser;
  readonly committer: CommitUser;
  readonly message: string;
  readonly treeDiff: CommitTreeDiff[];
}

export interface QueryBlamelistResponse {
  readonly commits: GitCommit[];
  readonly nextPageToken?: string;
  readonly precedingCommit?: GitCommit;
}

const SERVICE = 'luci.milo.v1.MiloInternal';

export class MiloInternal {
  private prpcClient: PrpcClient;

  constructor(accessToken: string) {
    this.prpcClient = new PrpcClient({host: '', accessToken});
  }

  async queryBlamelist(req: QueryBlamelistRequest) {
    return await this.call(
      'QueryBlamelist',
      req,
    ) as QueryBlamelistResponse;
  }

  private call(method: string, message: object) {
    return this.prpcClient.call(
      SERVICE,
      method,
      message,
    );
  }
}
