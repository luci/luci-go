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

import { DateTime } from 'luxon';

import {
  Commit,
  Commit_TreeDiff,
  Commit_User,
  commit_TreeDiff_ChangeTypeFromJSON,
} from '@/proto/go.chromium.org/luci/common/proto/git/commit.pb';
import { LogResponse } from '@/proto/go.chromium.org/luci/common/proto/gitiles/gitiles.pb';

export interface RestUser {
  readonly name?: string;
  readonly email?: string;
  readonly time?: string;
}

export const RestUser = {
  toProto(rest: RestUser): Commit_User {
    return {
      name: rest.name || '',
      email: rest.email || '',
      time: rest.time
        ? DateTime.fromFormat(rest.time, 'ccc LLL dd hh:mm:ss yyyy', {
            zone: 'UTC',
          }).toISO()
        : undefined,
    };
  },
};

export interface RestTreeDiff {
  readonly type?: 'add' | 'copy' | 'delete' | 'modify' | 'rename';
  readonly old_id?: string;
  readonly old_mode?: number;
  readonly old_path?: string;
  readonly new_id?: string;
  readonly new_mode?: number;
  readonly new_path?: string;
}

export const RestTreeDiff = {
  toProto(treeDiff: RestTreeDiff): Commit_TreeDiff {
    return {
      type: commit_TreeDiff_ChangeTypeFromJSON(treeDiff.type?.toUpperCase()),
      oldId: treeDiff.old_id || '',
      oldMode: treeDiff.old_mode || 0,
      oldPath: treeDiff.old_path || '',
      newId: treeDiff.new_id || '',
      newMode: treeDiff.new_mode || 0,
      newPath: treeDiff.new_path || '',
    };
  },
};

export interface RestCommit {
  readonly commit?: string;
  readonly tree?: string;
  readonly parents?: readonly string[];
  readonly author?: RestUser;
  readonly committer?: RestUser;
  readonly message?: string;
  readonly tree_diff?: readonly RestTreeDiff[];
}

export const RestCommit = {
  toProto(commit: RestCommit): Commit {
    return {
      id: commit.commit || '',
      tree: commit.tree || '',
      parents: commit.parents || [],
      author: commit.author ? RestUser.toProto(commit.author) : undefined,
      committer: commit.committer
        ? RestUser.toProto(commit.committer)
        : undefined,
      message: commit.message || '',
      treeDiff: commit.tree_diff
        ? commit.tree_diff.map((diff) => RestTreeDiff.toProto(diff))
        : [],
    };
  },
};

export interface RestLogResponse {
  readonly log?: readonly RestCommit[];
  readonly next?: string;
}

export const RestLogResponse = {
  toProto(res: RestLogResponse): LogResponse {
    return {
      log: res.log ? res.log.map((c) => RestCommit.toProto(c)) : [],
      nextPageToken: res.next || '',
    };
  },
};
