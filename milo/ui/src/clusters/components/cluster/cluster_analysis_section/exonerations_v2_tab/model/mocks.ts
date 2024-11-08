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

import { ExoneratedTestVariantBranch } from '@/clusters/hooks/use_fetch_exonerated_test_variant_branches';
import { DeepMutable } from '@/clusters/types/types';
import { ChangelistOwnerKind } from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import { TestStabilityCriteria } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

export const TestCriteria: TestStabilityCriteria = {
  failureRate: {
    failureThreshold: 7,
    consecutiveFailureThreshold: 3,
  },
  flakeRate: {
    minWindow: 100,
    flakeRateThreshold: 0.01,
    flakeThreshold: 2,
    flakeThreshold1wd: 0,
  },
};

export class ExoneratedTestVariantBranchBuilder {
  tvb: DeepMutable<ExoneratedTestVariantBranch>;
  constructor() {
    this.tvb = {
      testId: 'someTestId',
      variant: {
        def: {
          keya: 'valuea',
          keyb: 'valueb',
        },
      },
      sourceRef: {
        gitiles: {
          host: 'myproject.googlesource.com',
          project: 'myproject/src',
          ref: 'refs/heads/mybranch',
        },
      },
      lastExoneration: '2051-01-02T03:04:05.678901234Z',
      criticalFailuresExonerated: 100001,
      failureRate: {
        isMet: false,
        consecutiveUnexpectedTestRuns: 1,
        unexpectedTestRuns: 3,
        recentVerdicts: [
          {
            position: '9000000003',
            changelists: [
              {
                host: 'myprojectthree-review.googlesource.com',
                change: 'changelist-three',
                patchset: 8333,
                ownerKind: ChangelistOwnerKind.HUMAN,
              },
              {
                host: 'myprojectfour-review.googlesource.com',
                change: 'changelist-four',
                patchset: 8444,
                ownerKind: ChangelistOwnerKind.CHANGELIST_OWNER_UNSPECIFIED,
              },
            ],
            invocations: ['build-444444444444', 'build-55555555555'],
            unexpectedRuns: 1,
            totalRuns: 1,
          },
          {
            position: '9000000002',
            changelists: [],
            invocations: ['build-333333333333'],
            unexpectedRuns: 1,
            totalRuns: 9,
          },
        ],
      },
      flakeRate: {
        isMet: false,
        runFlakyVerdicts: 0,
        runFlakyVerdicts12h: 0,
        runFlakyVerdicts1wd: 0,
        totalVerdicts: 100,
        endPosition1wd: '0',
        startPosition1wd: '0',
        flakeExamples: [
          {
            position: '8000000005',
            invocations: ['build-222222222222', 'build-111111111111'],
            changelists: [
              {
                host: 'myprojectone-review.googlesource.com',
                change: 'changelist-one',
                patchset: 8111,
                ownerKind: ChangelistOwnerKind.HUMAN,
              },
              {
                host: 'myprojecttwo-review.googlesource.com',
                change: 'changelist-two',
                patchset: 8222,
                ownerKind: ChangelistOwnerKind.CHANGELIST_OWNER_UNSPECIFIED,
              },
            ],
          },
          {
            position: '8000000001',
            invocations: ['build-0000000000'],
            changelists: [],
          },
        ],
        startPosition: '8000000000',
        endPosition: '9000000100',
      },
    };
  }
  build(): DeepMutable<ExoneratedTestVariantBranch> {
    return this.tvb;
  }
  almostMeetsFlakyThreshold(): ExoneratedTestVariantBranchBuilder {
    this.tvb.flakeRate.runFlakyVerdicts = 1;
    return this;
  }
  meetsFlakyThreshold(): ExoneratedTestVariantBranchBuilder {
    this.tvb.flakeRate.isMet = true;
    this.tvb.flakeRate.runFlakyVerdicts = 2;
    return this;
  }
  almostMeetsFailureThreshold(): ExoneratedTestVariantBranchBuilder {
    this.tvb.failureRate.consecutiveUnexpectedTestRuns = 2;
    return this;
  }
  meetsFailureThreshold(): ExoneratedTestVariantBranchBuilder {
    this.tvb.failureRate.isMet = true;
    this.tvb.failureRate.consecutiveUnexpectedTestRuns = 3;
    return this;
  }
}
