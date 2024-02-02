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

import { BuilderID } from '@/proto/go.chromium.org/luci/buildbucket/proto/builder_common.pb';
import { GitilesCommit } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { getCommitShortHash } from './commit_formatters';

export const EMPTY_LINK: ExternalLink = {
  linkText: '',
  url: '',
};

export interface ExternalLink {
  linkText: string;
  url: string;
}

export const linkToBuild = (buildID: string): ExternalLink => {
  return {
    linkText: buildID,
    url: `https://ci.chromium.org/b/${buildID}`,
  };
};

export const linkToBuilder = (builderID: BuilderID): ExternalLink => {
  const { project, bucket, builder } = builderID;
  return {
    linkText: `${project}/${bucket}/${builder}`,
    url: `https://ci.chromium.org/p/${project}/builders/${bucket}/${builder}`,
  };
};

export const linkToCommit = (commit: GitilesCommit): ExternalLink => {
  const { host, project, id } = commit;
  return {
    linkText: getCommitShortHash(id),
    url: `https://${host}/${project}/+/${id}`,
  };
};

export const linkToCommitRange = (
  lastPassed: GitilesCommit,
  firstFailed: GitilesCommit,
): ExternalLink => {
  const host = lastPassed.host;
  const project = lastPassed.project;
  const lastPassedShortHash = getCommitShortHash(lastPassed.id);
  const firstFailedShortHash = getCommitShortHash(firstFailed.id);
  return {
    linkText: `${lastPassedShortHash} ... ${firstFailedShortHash}`,
    url: `https://${host}/${project}/+log/${lastPassed.id}..${firstFailed.id}`,
  };
};
