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


import { BuilderID, GitilesCommit } from '../services/luci_bisection';

export interface ExternalLink {
  linkText: string;
  url: string;
}

export const linkToBuild = (buildID: string) => {
  return {
    linkText: buildID,
    url: `https://ci.chromium.org/b/${buildID}`,
  };
};

export const linkToBuilder = ({ project, bucket, builder }: BuilderID) => {
  return {
    linkText: `${project}/${bucket}/${builder}`,
    url: `https://ci.chromium.org/p/${project}/builders/${bucket}/${builder}`,
  };
};

export const linkToCommit = (commit: GitilesCommit) => {
  return {
    linkText: getCommitShortHash(commit.id),
    url: `https://${commit.host}/${commit.project}/+log/${commit.id}$`,
  };
};

export const linkToCommitRange = (
  lastPassed: GitilesCommit,
  firstFailed: GitilesCommit
) => {
  const host = lastPassed.host;
  const project = lastPassed.project;
  const lastPassedShortHash = getCommitShortHash(lastPassed.id);
  const firstFailedShortHash = getCommitShortHash(firstFailed.id);
  return {
    linkText: `${lastPassedShortHash} ... ${firstFailedShortHash}`,
    url: `https://${host}/${project}/+log/${lastPassedShortHash}..${firstFailedShortHash}`,
  };
};

export const getCommitShortHash = (commitID: string) => {
  return commitID.substring(0, 7);
};
