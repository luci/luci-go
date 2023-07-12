// Copyright 2023 The LUCI Authors.
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

import { GitilesCommit } from '@/common/services/buildbucket';

export function getGitilesRepoURL(
  commit: Pick<GitilesCommit, 'host' | 'project'>
) {
  return `https://${commit.host}/${commit.project}`;
}

export function getGitilesCommitURL(commit: GitilesCommit): string {
  return `${getGitilesRepoURL(commit)}/+/${commit.id}`;
}

export function getGitilesCommitLabel(
  commit: Pick<GitilesCommit, 'id' | 'position' | 'ref'>
): string {
  if (commit.position && commit.ref) {
    return `${commit.ref}@{#${commit.position}}`;
  }

  if (commit.ref?.startsWith('refs/tags/')) {
    return commit.ref;
  }

  if (commit.id) {
    return commit.id.slice(0, 9);
  }

  if (commit.ref) {
    return commit.ref;
  }

  return '';
}
