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

import { DateTime } from 'luxon';

import {
  FailureGroup,
  GroupKey,
  ImpactFilter,
  ImpactFilters,
  VariantGroup,
} from '@/clusters/tools/failures_tools';
import { Mutable } from '@/clusters/types/types';
import { DistinctClusterFailure } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import {
  BuildStatus,
  ExonerationReason,
  PresubmitRunMode,
  PresubmitRunStatus,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import { ChangelistOwnerKind } from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';

class ClusterFailureBuilder {
  failure: Mutable<DistinctClusterFailure>;
  constructor() {
    this.failure = {
      testId: 'ninja://dir/test.param',
      variant: undefined,
      presubmitRun: {
        presubmitRunId: { system: 'cv', id: 'presubmitRunId' },
        owner: 'user',
        mode: PresubmitRunMode.FULL_RUN,
        status: PresubmitRunStatus.PRESUBMIT_RUN_STATUS_SUCCEEDED,
      },
      changelists: [
        {
          host: 'clproject-review.googlesource.com',
          change: '123456',
          patchset: 7,
          ownerKind: ChangelistOwnerKind.CHANGELIST_OWNER_UNSPECIFIED,
        },
      ],
      partitionTime: '2021-05-12T19:05:34',
      exonerations: [],
      buildStatus: BuildStatus.BUILD_STATUS_SUCCESS,
      isBuildCritical: true,
      ingestedInvocationId: 'build-buildnumber',
      isIngestedInvocationBlocked: false,
      count: 1,
      failureReasonPrefix: 'failed due to timeout',
    };
  }
  build(): DistinctClusterFailure {
    return this.failure;
  }
  ingestedInvocationBlocked() {
    this.failure.isIngestedInvocationBlocked = true;
    return this;
  }
  notPresubmitCritical() {
    this.failure.isBuildCritical = false;
    return this;
  }
  buildFailed() {
    this.failure.buildStatus = BuildStatus.BUILD_STATUS_FAILURE;
    return this;
  }
  dryRun() {
    this.failure.presubmitRun = {
      presubmitRunId: { system: 'cv', id: 'presubmitRunId' },
      owner: 'user',
      mode: PresubmitRunMode.DRY_RUN,
      status: PresubmitRunStatus.PRESUBMIT_RUN_STATUS_SUCCEEDED,
    };
    return this;
  }
  exonerateOccursOnOtherCLs() {
    this.failure.exonerations = [
      { reason: ExonerationReason.OCCURS_ON_OTHER_CLS },
    ];
    return this;
  }
  exonerateNotCritical() {
    this.failure.exonerations = [{ reason: ExonerationReason.NOT_CRITICAL }];
    return this;
  }
  withVariantGroups(key: string, value: string) {
    if (this.failure.variant === undefined) {
      this.failure.variant = { def: {} };
    }
    this.failure.variant.def[key] = value;
    return this;
  }
  withTestId(id: string) {
    this.failure.testId = id;
    return this;
  }
  withoutPresubmit() {
    this.failure.changelists = [];
    this.failure.presubmitRun = undefined;
    return this;
  }
}

export const newMockGroup = (key: GroupKey): FailureGroupBuilder => {
  return new FailureGroupBuilder(key);
};

class FailureGroupBuilder {
  failureGroup: FailureGroup;
  constructor(key: GroupKey) {
    this.failureGroup = {
      id: key.value,
      key,
      children: [],
      criticalFailuresExonerated: 0,
      failures: 0,
      invocationFailures: 0,
      presubmitRejects: 0,
      latestFailureTime: DateTime.now().toISO(),
      isExpanded: false,
      level: 0,
      failure: undefined,
    };
  }

  build(): FailureGroup {
    return this.failureGroup;
  }

  withFailures(failures: number) {
    this.failureGroup.failures = failures;
    return this;
  }

  withPresubmitRejects(presubmitRejects: number) {
    this.failureGroup.presubmitRejects = presubmitRejects;
    return this;
  }

  withCriticalFailuresExonerated(criticalFailuresExonerated: number) {
    this.failureGroup.criticalFailuresExonerated = criticalFailuresExonerated;
    return this;
  }

  withInvocationFailures(invocationFailures: number) {
    this.failureGroup.invocationFailures = invocationFailures;
    return this;
  }

  withFailure(failure: DistinctClusterFailure) {
    this.failureGroup.failure = failure;
    return this;
  }

  withChildren(children: FailureGroup[]) {
    this.failureGroup.children = children;
    return this;
  }
}

// Helper functions.
export const impactFilterNamed = (name: string) => {
  return ImpactFilters.filter((f: ImpactFilter) => name === f.name)?.[0];
};

export const newMockFailure = (): ClusterFailureBuilder => {
  return new ClusterFailureBuilder();
};

export const createDefaultMockFailure = (): DistinctClusterFailure => {
  return newMockFailure().build();
};

export const createDefaultMockFailures = (
  num = 5,
): Array<DistinctClusterFailure> => {
  return Array.from(Array(num).keys()).map(() => createDefaultMockFailure());
};

export const createMockSelectedVariantGroups = (): string[] => {
  return Array.from(Array(4).keys()).map((k) => `v${k}`);
};

export const createMockVariantGroups = (): VariantGroup[] => {
  return Array.from(Array(4).keys()).map((k) => ({
    key: `v${k}`,
    values: [`value${k}`],
    isSelected: false,
  }));
};

export const createDefaultMockFailureGroup = (
  key: GroupKey | null = null,
): FailureGroup => {
  if (!key) {
    key = { type: 'test', value: 'testgroup' };
  }
  return newMockGroup(key).withFailures(1).build();
};

export const createDefaultMockFailureGroupWithChildren = (): FailureGroup => {
  return newMockGroup({ type: 'test', value: 'testgroup' })
    .withChildren([
      newMockGroup({ type: 'leaf', value: 'a3' }).withFailures(3).build(),
      newMockGroup({ type: 'leaf', value: 'a2' }).withFailures(2).build(),
      newMockGroup({ type: 'leaf', value: 'a1' }).withFailures(1).build(),
    ])
    .build();
};
