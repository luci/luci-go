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
import { BuilderID, GitilesCommit, Timestamp } from './buildbucket';

/**
 * Manually coded type definition and classes for milo internal service.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source: https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/master/milo/api/service/v1/rpc.proto
 */

export interface QueryBlamelistRequest {
  readonly gitiles_commit: GitilesCommit;
  readonly builder: BuilderID;
  readonly page_size?: number;
  readonly page_token?: string;
}

export interface CommitUser {
  readonly name: string;
  readonly email: string;
  readonly time: Timestamp;
}

export const enum CommitTreeDiffChangeType {
  Add = 0,
  Copy = 1,
  Delete = 2,
  Modify = 3,
  Rename = 4,
}

export interface CommitTreeDiff {
  readonly type: CommitTreeDiffChangeType;
  readonly old_id: string;
  readonly old_mode: number;
  readonly old_path: string;
  readonly new_id: string;
  readonly new_mode: number;
  readonly new_path: string;
}

export interface GitCommit {
  readonly id: string;
  readonly tree: string;
  readonly parents?: string[];
  readonly author: CommitTreeDiffChangeType;
  readonly commiter: CommitTreeDiffChangeType;
  readonly message: string;
  readonly tree_diff: CommitTreeDiff[];
}

export interface QueryBlamelistResponse {
  readonly commits: GitCommit[];
  readonly next_page_token?: string;
}

const SERVICE = 'luci.milo.v1.MiloInternal';

export class MiloInternal {
  private prpcClient: PrpcClient;

  constructor(accessToken: string) {
    this.prpcClient = new PrpcClient({host: '', accessToken, insecure: true});
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
