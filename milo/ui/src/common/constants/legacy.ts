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

/**
 * @fileoverview
 * The constants defined in this file rely on old hand-written protobuf types.
 * They should be replaced by equivalent constants that supports the generated
 * protobuf types overtime.
 */

import { fromPromise } from 'mobx-utils';

import { BuildbucketStatus } from '@/common/services/buildbucket';
import { TestVerdictStatus } from '@/common/services/luci_analysis';
import {
  InvocationState,
  TestStatus,
  TestVariantStatus,
} from '@/common/services/resultdb';

import { NEVER_PROMISE } from './utils';

export const INVOCATION_STATE_DISPLAY_MAP = {
  [InvocationState.Unspecified]: 'unspecified',
  [InvocationState.Active]: 'active',
  [InvocationState.Finalizing]: 'finalizing',
  [InvocationState.Finalized]: 'finalized',
};

// Just so happens to be the same as VARIANT_STATUS_DISPLAY_MAP.
export const VARIANT_STATUS_CLASS_MAP = Object.freeze({
  [TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED]: 'unspecified',
  [TestVariantStatus.UNEXPECTED]: 'unexpected',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 'unexpectedly-skipped',
  [TestVariantStatus.FLAKY]: 'flaky',
  [TestVariantStatus.EXONERATED]: 'exonerated',
  [TestVariantStatus.EXPECTED]: 'expected',
});

export const VARIANT_STATUS_ICON_MAP = Object.freeze({
  [TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED]: 'unspecified',
  [TestVariantStatus.UNEXPECTED]: 'error',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 'report',
  [TestVariantStatus.FLAKY]: 'warning',
  [TestVariantStatus.EXONERATED]: 'remove_circle',
  [TestVariantStatus.EXPECTED]: 'check_circle',
});

export const VERDICT_VARIANT_STATUS_MAP = Object.freeze({
  [TestVerdictStatus.TEST_VERDICT_STATUS_UNSPECIFIED]:
    TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED,
  [TestVerdictStatus.UNEXPECTED]: TestVariantStatus.UNEXPECTED,
  [TestVerdictStatus.UNEXPECTEDLY_SKIPPED]:
    TestVariantStatus.UNEXPECTEDLY_SKIPPED,
  [TestVerdictStatus.FLAKY]: TestVariantStatus.FLAKY,
  [TestVerdictStatus.EXONERATED]: TestVariantStatus.EXONERATED,
  [TestVerdictStatus.EXPECTED]: TestVariantStatus.EXPECTED,
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
  [BuildbucketStatus.Scheduled]: 'scheduled',
  [BuildbucketStatus.Started]: 'running',
  [BuildbucketStatus.Success]: 'succeeded',
  [BuildbucketStatus.Failure]: 'failed',
  [BuildbucketStatus.InfraFailure]: 'infra failed',
  [BuildbucketStatus.Canceled]: 'canceled',
});

export const BUILD_STATUS_CLASS_MAP = Object.freeze({
  [BuildbucketStatus.Scheduled]: 'scheduled',
  [BuildbucketStatus.Started]: 'started',
  [BuildbucketStatus.Success]: 'success',
  [BuildbucketStatus.Failure]: 'failure',
  [BuildbucketStatus.InfraFailure]: 'infra-failure',
  [BuildbucketStatus.Canceled]: 'canceled',
});

export const BUILD_STATUS_ICON_MAP = Object.freeze({
  [BuildbucketStatus.Scheduled]: 'schedule',
  [BuildbucketStatus.Started]: 'timelapse',
  [BuildbucketStatus.Success]: 'check_circle',
  [BuildbucketStatus.Failure]: 'error',
  [BuildbucketStatus.InfraFailure]: 'report',
  [BuildbucketStatus.Canceled]: 'cancel',
});

export const BUILD_STATUS_COLOR_MAP = Object.freeze({
  [BuildbucketStatus.Scheduled]: 'var(--scheduled-color)',
  [BuildbucketStatus.Started]: 'var(--started-color)',
  [BuildbucketStatus.Success]: 'var(--success-color)',
  [BuildbucketStatus.Failure]: 'var(--failure-color)',
  [BuildbucketStatus.InfraFailure]: 'var(--critical-failure-color)',
  [BuildbucketStatus.Canceled]: 'var(--canceled-color)',
});

export const BUILD_STATUS_COLOR_THEME_MAP = Object.freeze({
  [BuildbucketStatus.Scheduled]: 'scheduled',
  [BuildbucketStatus.Started]: 'started',
  [BuildbucketStatus.Success]: 'success',
  [BuildbucketStatus.Failure]: 'error',
  [BuildbucketStatus.InfraFailure]: 'criticalFailure',
  [BuildbucketStatus.Canceled]: 'canceled',
});

/**
 * Some resources are purged from the server after certain period (e.g. builds
 * are removed from buildbucket server after 1.5 years). 404 errors that ocurred
 * when querying those resources should have this tag attached to them.
 */
export const POTENTIALLY_EXPIRED = Symbol('POTENTIALLY_EXPIRED');

export const NEVER_OBSERVABLE = fromPromise(NEVER_PROMISE);
