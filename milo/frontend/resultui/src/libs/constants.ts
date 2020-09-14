// Copyright 2020 The LUCI Authors.
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

import { VariantStatus } from '../models/test_node';
import { BuildStatus } from '../services/buildbucket';
import { InvocationState, TestStatus } from '../services/resultdb';

export const INVOCATION_STATE_DISPLAY_MAP = {
  [InvocationState.Unspecified]: 'unspecified',
  [InvocationState.Active]: 'active',
  [InvocationState.Finalizing]: 'finalizing',
  [InvocationState.Finalized]: 'finalized',
};


export const VARIANT_STATUS_DISPLAY_MAP = Object.freeze({
  [VariantStatus.Exonerated]: 'exonerated',
  [VariantStatus.Expected]: 'expected',
  [VariantStatus.Unexpected]: 'unexpected',
  [VariantStatus.Flaky]: 'flaky',
});

// Just so happens to be the same as VARIANT_STATUS_DISPLAY_MAP.
export const VARIANT_STATUS_CLASS_MAP = Object.freeze({
  [VariantStatus.Exonerated]: 'exonerated',
  [VariantStatus.Expected]: 'expected',
  [VariantStatus.Unexpected]: 'unexpected',
  [VariantStatus.Flaky]: 'flaky',
});

export const VARIANT_STATUS_ICON_MAP = Object.freeze({
  // TODO(weiweilin): find an appropriate icon for exonerated
  [VariantStatus.Exonerated]: 'check',
  [VariantStatus.Expected]: 'check',
  [VariantStatus.Flaky]: 'warning',
  [VariantStatus.Unexpected]: 'error',
});


export const TEST_STATUS_DISPLAY_MAP = Object.freeze({
  [TestStatus.Unspecified]: 'unspecified',
  [TestStatus.Pass]: 'passed',
  [TestStatus.Fail]: 'failed',
  [TestStatus.Skip]: 'skipped',
  [TestStatus.Crash]: 'crashed',
  [TestStatus.Abort]: 'aborted',
});


export const BUILD_STATUS_DISPLAY_MAP = Object.freeze({
  [BuildStatus.Scheduled]: 'scheduled',
  [BuildStatus.Started]: 'started',
  [BuildStatus.Success]: 'succeeded',
  [BuildStatus.Failure]: 'failed',
  [BuildStatus.InfraFailure]: 'infra-failed',
  [BuildStatus.Canceled]: 'canceled',
});

// Just so happens to be the same as BUILD_STATUS_DISPLAY_MAP.
export const BUILD_STATUS_CLASS_MAP = Object.freeze({
  [BuildStatus.Scheduled]: 'scheduled',
  [BuildStatus.Started]: 'started',
  [BuildStatus.Success]: 'success',
  [BuildStatus.Failure]: 'failure',
  [BuildStatus.InfraFailure]: 'infra-failure',
  [BuildStatus.Canceled]: 'canceled',
});

export const BUILD_STATUS_ICON_MAP = Object.freeze({
  [BuildStatus.Scheduled]: 'schedule',
  [BuildStatus.Started]: 'pending',
  [BuildStatus.Success]: 'check',
  [BuildStatus.Failure]: 'error',
  [BuildStatus.InfraFailure]: 'report',
  [BuildStatus.Canceled]: 'cancel',
});
