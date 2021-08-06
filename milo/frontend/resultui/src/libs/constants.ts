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

import { BuildStatus } from '../services/buildbucket';
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
