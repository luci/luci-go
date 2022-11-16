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

import { fromPromise } from 'mobx-utils';

import { BuildStatus } from '../services/buildbucket';
import { TestVerdictStatus } from '../services/luci_analysis';
import { InvocationState, TestStatus, TestVariantStatus } from '../services/resultdb';

export const NEW_MILO_VERSION_EVENT_TYPE = 'new-milo-version';

export const INVOCATION_STATE_DISPLAY_MAP = {
  [InvocationState.Unspecified]: 'unspecified',
  [InvocationState.Active]: 'active',
  [InvocationState.Finalizing]: 'finalizing',
  [InvocationState.Finalized]: 'finalized',
};

export const VARIANT_STATUS_DISPLAY_MAP = Object.freeze({
  [TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED]: 'unspecified',
  [TestVariantStatus.UNEXPECTED]: 'unexpected',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 'unexpectedly skipped',
  [TestVariantStatus.FLAKY]: 'flaky',
  [TestVariantStatus.EXONERATED]: 'exonerated',
  [TestVariantStatus.EXPECTED]: 'expected',
});

export const VARIANT_STATUS_DISPLAY_MAP_TITLE_CASE = Object.freeze({
  [TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED]: 'Unspecified',
  [TestVariantStatus.UNEXPECTED]: 'Unexpected',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: 'Unexpectedly Skipped',
  [TestVariantStatus.FLAKY]: 'Flaky',
  [TestVariantStatus.EXONERATED]: 'Exonerated',
  [TestVariantStatus.EXPECTED]: 'Expected',
});

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
  [TestVerdictStatus.TEST_VERDICT_STATUS_UNSPECIFIED]: TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED,
  [TestVerdictStatus.UNEXPECTED]: TestVariantStatus.UNEXPECTED,
  [TestVerdictStatus.UNEXPECTEDLY_SKIPPED]: TestVariantStatus.UNEXPECTEDLY_SKIPPED,
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
  [BuildStatus.Scheduled]: 'scheduled',
  [BuildStatus.Started]: 'running',
  [BuildStatus.Success]: 'succeeded',
  [BuildStatus.Failure]: 'failed',
  [BuildStatus.InfraFailure]: 'infra failed',
  [BuildStatus.Canceled]: 'canceled',
});

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
  [BuildStatus.Success]: 'check_circle',
  [BuildStatus.Failure]: 'error',
  [BuildStatus.InfraFailure]: 'report',
  [BuildStatus.Canceled]: 'cancel',
});

export const BUILD_STATUS_COLOR_MAP = Object.freeze({
  [BuildStatus.Scheduled]: 'var(--scheduled-color)',
  [BuildStatus.Started]: 'var(--started-color)',
  [BuildStatus.Success]: 'var(--success-color)',
  [BuildStatus.Failure]: 'var(--failure-color)',
  [BuildStatus.InfraFailure]: 'var(--critical-failure-color)',
  [BuildStatus.Canceled]: 'var(--canceled-color)',
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

export const NEVER_PROMISE = new Promise<never>(() => {});
export const NEVER_OBSERVABLE = fromPromise(NEVER_PROMISE);
