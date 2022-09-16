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

import {
  Changelist,
  DistinctClusterFailure,
} from '@/services/cluster';
import { ClusterId } from '@/services/shared_models';

export const linkToCluster = (project: string, c: ClusterId): string => {
  if (c.algorithm.startsWith('rules-') || c.algorithm == 'rules') {
    return linkToRule(project, c.id);
  } else {
    const projectEncoded = encodeURIComponent(project);
    const algorithmEncoded = encodeURIComponent(c.algorithm);
    const idEncoded = encodeURIComponent(c.id);
    return `/p/${projectEncoded}/clusters/${algorithmEncoded}/${idEncoded}`;
  }
};

export const linkToRule = (project: string, ruleId: string): string => {
  const projectEncoded = encodeURIComponent(project);
  const ruleIdEncoded = encodeURIComponent(ruleId);
  return `/p/${projectEncoded}/rules/${ruleIdEncoded}`;
};

export const failureLink = (failure: DistinctClusterFailure) => {
  const query = `ID:${failure.testId} `;
  if (failure.ingestedInvocationId?.startsWith('build-')) {
    return `https://ci.chromium.org/ui/b/${failure.ingestedInvocationId.replace('build-', '')}/test-results?q=${encodeURIComponent(query)}`;
  }
  return `https://ci.chromium.org/ui/inv/${failure.ingestedInvocationId}/test-results?q=${encodeURIComponent(query)}`;
};

export const clLink = (cl: Changelist) => {
  return `https://${cl.host}/c/${cl.change}/${cl.patchset}`;
};

export const clName = (cl: Changelist) => {
  const host = cl.host.replace('-review.googlesource.com', '');
  return `${host}/${cl.change}/${cl.patchset}`;
};

export const testHistoryLink = (project: string, testId: string, query: string) => {
  return `https://ci.chromium.org/ui/test/${encodeURIComponent(project)}/${encodeURIComponent(testId)}?q=${encodeURIComponent(query)}`;
};
