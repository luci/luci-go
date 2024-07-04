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

import {
  GerritChange,
  GitilesCommit,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

const GITILES_HOST_RE = /^(?<gitilesProject>[^.]+).googlesource[.]com$/;

/**
 * Extract the host project from a gitiles host.
 *
 * For example, the host project of `chromium.googlesource.com` is `chromium`.
 */
export function getGitilesHostProject(host: string) {
  const match = GITILES_HOST_RE.exec(host);
  if (!match) {
    throw new Error(`${host} is not a valid Gitiles host`);
  }
  return match.groups!['gitilesProject'];
}

export function getGitilesHostURL(commit: Pick<GitilesCommit, 'host'>) {
  return `https://${commit.host}`;
}

export function getGitilesRepoURL(
  commit: Pick<GitilesCommit, 'host' | 'project'>,
) {
  return `${getGitilesHostURL(commit)}/${commit.project}`;
}

export function getGitilesCommitURL(
  commit:
    | Pick<GitilesCommit, 'host' | 'project' | 'ref'>
    | Pick<GitilesCommit, 'host' | 'project' | 'id'>,
): string {
  const commitish =
    ('id' in commit && commit.id) || ('ref' in commit && commit.ref);
  return `${getGitilesRepoURL(commit)}/+/${commitish}`;
}

export function getGitilesCommitLabel(
  commit: Pick<GitilesCommit, 'id' | 'position' | 'ref'>,
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

export function getGerritChangeURL(change: GerritChange): string {
  return `https://${change.host}/c/${change.change}/${change.patchset}`;
}
