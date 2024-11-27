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
  ClusterId,
  PresubmitRunId,
  Variant,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import {
  Changelist,
  SourceRef,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';

import { variantAsPairs } from '../variant_tools';

export const linkToCluster = (project: string, c: ClusterId): string => {
  if (c.algorithm.startsWith('rules-') || c.algorithm === 'rules') {
    return linkToRule(project, c.id);
  } else {
    const projectEncoded = encodeURIComponent(project);
    const algorithmEncoded = encodeURIComponent(c.algorithm);
    const idEncoded = encodeURIComponent(c.id);
    return `/ui/tests/p/${projectEncoded}/clusters/${algorithmEncoded}/${idEncoded}`;
  }
};

export const linkToRule = (project: string, ruleId: string): string => {
  const projectEncoded = encodeURIComponent(project);
  const ruleIdEncoded = encodeURIComponent(ruleId);
  return `/ui/tests/p/${projectEncoded}/rules/${ruleIdEncoded}`;
};

export const invocationName = (invocationId: string): string => {
  if (invocationId.startsWith('build-')) {
    return invocationId.slice('build-'.length);
  }
  return invocationId;
};

export const failureLink = (
  invocationId: string,
  testId: string,
  variant?: Variant,
): string => {
  const variantQuery = variantAsPairs(variant)
    .map((vp) => {
      return (
        'V:' +
        encodeURIComponent(vp.key || '') +
        '=' +
        encodeURIComponent(vp.value)
      );
    })
    .join(' ');
  const query = `ID:${testId} ${variantQuery}`;
  if (invocationId.startsWith('build-')) {
    return `/ui/b/${invocationId.slice('build-'.length)}/test-results?q=${encodeURIComponent(query)}`;
  }
  return `/ui/inv/${invocationId}/test-results?q=${encodeURIComponent(query)}`;
};

export const clLink = (cl: Changelist): string => {
  return `https://${cl.host}/c/${cl.change}/${cl.patchset}`;
};

export const clName = (cl: Changelist): string => {
  const host = cl.host.replace('-review.googlesource.com', '');
  return `${host}/${cl.change}/${cl.patchset}`;
};

export const presubmitRunLink = (runId: PresubmitRunId): string => {
  return `https://luci-change-verifier.appspot.com/ui/run/${runId.id}`;
};

export const testHistoryLink = (
  project: string,
  testId: string,
  partialVariant?: Variant,
): string => {
  const query = variantAsPairs(partialVariant)
    .map((vp) => {
      return (
        'V:' +
        encodeURIComponent(vp.key || '') +
        '=' +
        encodeURIComponent(vp.value)
      );
    })
    .join(' ');
  return `/ui/test/${encodeURIComponent(project)}/${encodeURIComponent(testId)}?q=${encodeURIComponent(query)}`;
};

export const sourceRefLink = (sourceRef: SourceRef): string => {
  if (sourceRef.gitiles) {
    return `https://${sourceRef.gitiles.host}/${sourceRef.gitiles.project}/+log/${sourceRef.gitiles.ref}`;
  }
  return '';
};
