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

import { ChangelistOwnerKind } from '@/proto/go.chromium.org/luci/analysis/proto/v1/sources.pb';
import { ExoneratedTestVariant } from './model';

export class ExoneratedTestVariantBuilder {
  tv: ExoneratedTestVariant;
  constructor() {
    this.tv = {
      key: 'someKey',
      testId: 'someTestId',
      variant: {
        def: {
          'keya': 'valuea',
          'keyb': 'valueb',
        },
      },
      lastExoneration: '2051-01-02T03:04:05.678901234Z',
      criticalFailuresExonerated: 100001,
      runFlakyVerdicts1wd: 0,
      runFlakyVerdicts5wd: 1,
      runFlakyPercentage1wd: 2,
      recentVerdictsWithUnexpectedRuns: 2,
      runFlakyVerdictExamples: [{
        partitionTime: '2040-01-02T03:04:05.678901234Z',
        ingestedInvocationId: 'build-111111111111',
        changelists: [{
          host: 'myprojectone-review.googlesource.com',
          change: 'changelist-one',
          patchset: 8111,
          ownerKind: ChangelistOwnerKind.HUMAN,
        }, {
          host: 'myprojecttwo-review.googlesource.com',
          change: 'changelist-two',
          patchset: 8222,
          ownerKind: ChangelistOwnerKind.CHANGELIST_OWNER_UNSPECIFIED,
        }],
      }, {
        partitionTime: '2040-01-02T03:04:05.678901234Z',
        ingestedInvocationId: 'build-222222222222',
        changelists: [],
      }],
      recentVerdicts: [{
        partitionTime: '2030-01-02T03:04:05.678901234Z',
        ingestedInvocationId: 'build-333333333333',
        hasUnexpectedRuns: true,
        changelists: [{
          host: 'myprojectthree-review.googlesource.com',
          change: 'changelist-three',
          patchset: 8333,
          ownerKind: ChangelistOwnerKind.HUMAN,
        }, {
          host: 'myprojectfour-review.googlesource.com',
          change: 'changelist-four',
          patchset: 8444,
          ownerKind: ChangelistOwnerKind.CHANGELIST_OWNER_UNSPECIFIED,
        }],
      }, {
        partitionTime: '2030-01-02T03:04:05.678901234Z',
        ingestedInvocationId: 'build-444444444444',
        hasUnexpectedRuns: false,
        changelists: [],
      }],
    };
  }
  build(): ExoneratedTestVariant {
    return this.tv;
  }
  almostMeetsFlakyThreshold(): ExoneratedTestVariantBuilder {
    this.tv.runFlakyVerdicts5wd = 2;
    return this;
  }
  meetsFlakyThreshold(): ExoneratedTestVariantBuilder {
    this.tv.runFlakyVerdicts1wd = 1;
    this.tv.runFlakyVerdicts5wd = 3;
    return this;
  }
  almostMeetsFailureThreshold(): ExoneratedTestVariantBuilder {
    this.tv.recentVerdictsWithUnexpectedRuns = 5;
    return this;
  }
  meetsFailureThreshold(): ExoneratedTestVariantBuilder {
    this.tv.recentVerdictsWithUnexpectedRuns = 7;
    return this;
  }
}
