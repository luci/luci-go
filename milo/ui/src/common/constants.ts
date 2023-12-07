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

import { RpcCode } from '@chopsui/prpc-client';
import { fromPromise } from 'mobx-utils';

import { BuildbucketStatus } from '@/common/services/buildbucket';
import { TestVerdictStatus } from '@/common/services/luci_analysis';
import {
  InvocationState,
  TestStatus,
  TestVariantStatus,
} from '@/common/services/resultdb';

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
  [BuildbucketStatus.Started]: 'pending',
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

export const ARTIFACT_LENGTH_LIMIT = 50000;

/**
 * Some resources are purged from the server after certain period (e.g. builds
 * are removed from buildbucket server after 1.5 years). 404 errors that ocurred
 * when querying those resources should have this tag attached to them.
 */
export const POTENTIALLY_EXPIRED = Symbol('POTENTIALLY_EXPIRED');

/**
 * The number of milliseconds in a second.
 */
export const SECOND_MS = 1000;

/**
 * The number of milliseconds in a minute.
 */
export const MINUTE_MS = 60000;

/**
 * The number of milliseconds in an hour.
 */
export const HOUR_MS = 3600000;

/**
 * A list of numbers in ascending order that are suitable to be used as time
 * intervals (in ms).
 */
export const PREDEFINED_TIME_INTERVALS = Object.freeze([
  // Values that can divide 10ms.
  1, // 1ms
  5, // 5ms
  10, // 10ms
  // Values that can divide 100ms.
  20, // 20ms
  25, // 25ms
  50, // 50ms
  // Values that can divide 1 second.
  100, // 100ms
  125, // 125ms
  200, // 200ms
  250, // 250ms
  500, // 500ms
  // Values that can divide 15 seconds.
  1000, // 1s
  2000, // 2s
  3000, // 3s
  5000, // 5s
  // Values that can divide 1 minute.
  10000, // 10s
  15000, // 15s
  20000, // 20s
  30000, // 30s
  60000, // 1min
  // Values that can divide 15 minutes.
  120000, // 2min
  180000, // 3min
  300000, // 5min
  // Values that can divide 1 hour.
  600000, // 10min
  900000, // 15min
  1200000, // 20min
  1800000, // 30min
  3600000, // 1hr
  // Values that can divide 12 hours.
  7200000, // 2hr
  10800000, // 3hr
  14400000, // 4hr
  21600000, // 6hr
  // Values that can divide 1 day.
  28800000, // 8hr
  43200000, // 12hr
  86400000, // 24hr
]);

export const NEVER_PROMISE = new Promise<never>(() => {
  /* Never resolves. */
});
export const NEVER_OBSERVABLE = fromPromise(NEVER_PROMISE);

/**
 * A list of RPC Error code that MAY indicate the user lacks permission.
 */
export const POTENTIAL_PERM_ERROR_CODES = Object.freeze([
  // Some RPCs will return NOT_FOUND when user isn't able to access it.
  RpcCode.NOT_FOUND,
  RpcCode.PERMISSION_DENIED,
  RpcCode.UNAUTHENTICATED,
]);

/**
 * A list of RPC Error code that indicates the error is non-transient.
 */
export const NON_TRANSIENT_ERROR_CODES = Object.freeze([
  RpcCode.INVALID_ARGUMENT,
  RpcCode.PERMISSION_DENIED,
  RpcCode.UNAUTHENTICATED,
  RpcCode.UNIMPLEMENTED,
]);

/*
 * An enum representing the pages
 */
export enum UiPage {
  BuilderSearch = 'builder-search',
  ProjectSearch = 'project-search',
  Builders = 'builders',
  Scheduler = 'scheduler',
  Bisection = 'bisection',
  TestHistory = 'test-history',
  FailureClusters = 'failure-clusters',
  Testhaus = 'test-haus',
  Crosbolt = 'crosbolt',
  BuilderGroups = 'builder-groups',
  SoM = 'som',
  CQStatus = 'cq-status',
  Goldeneye = 'goldeneye',
  ChromiumDash = 'chromium-dash',
  ReleaseNotes = 'release-notes',
  TestVerdict = 'test-verdict',
}

export enum CommonColors {
  FADED_TEXT = '#888',
}
