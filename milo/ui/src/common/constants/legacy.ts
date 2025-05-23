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
import {
  TestResult_Status,
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/common/services/luci_analysis';
import { InvocationState, WebTest_Status } from '@/common/services/resultdb';

import { NEVER_PROMISE } from './utils';

export const INVOCATION_STATE_DISPLAY_MAP = {
  [InvocationState.Unspecified]: 'unspecified',
  [InvocationState.Active]: 'active',
  [InvocationState.Finalizing]: 'finalizing',
  [InvocationState.Finalized]: 'finalized',
};

export const VERDICT_STATUS_CLASS_MAP = Object.freeze({
  // Invalid values.
  [TestVerdict_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [TestVerdict_StatusOverride.STATUS_OVERRIDE_UNSPECIFIED]: 'unspecified',
  [TestVerdict_StatusOverride.NOT_OVERRIDDEN]: 'unspecified',
  // Valid values.
  [TestVerdict_Status.FAILED]: 'failed-verdict',
  [TestVerdict_Status.EXECUTION_ERRORED]: 'execution-errored-verdict',
  [TestVerdict_Status.PRECLUDED]: 'precluded-verdict',
  [TestVerdict_Status.PASSED]: 'passed-verdict',
  [TestVerdict_Status.SKIPPED]: 'skipped-verdict',
  [TestVerdict_Status.FLAKY]: 'flaky-verdict',
  [TestVerdict_StatusOverride.EXONERATED]: 'exonerated-verdict',
});

export const VERDICT_STATUS_ICON_MAP = Object.freeze({
  // Invalid values.
  [TestVerdict_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [TestVerdict_StatusOverride.STATUS_OVERRIDE_UNSPECIFIED]: 'unspecified',
  [TestVerdict_StatusOverride.NOT_OVERRIDDEN]: 'unspecified',
  // Valid values.
  [TestVerdict_Status.FAILED]: 'error',
  [TestVerdict_Status.EXECUTION_ERRORED]: 'report',
  [TestVerdict_Status.PRECLUDED]: 'not_started',
  [TestVerdict_Status.PASSED]: 'check_circle',
  [TestVerdict_Status.SKIPPED]: 'next_plan',
  [TestVerdict_Status.FLAKY]: 'warning',
  [TestVerdict_StatusOverride.EXONERATED]: 'remove_circle',
});

export const TEST_STATUS_V2_DISPLAY_MAP = Object.freeze({
  [TestResult_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [TestResult_Status.FAILED]: 'failed',
  [TestResult_Status.PASSED]: 'passed',
  [TestResult_Status.SKIPPED]: 'skipped',
  [TestResult_Status.EXECUTION_ERRORED]: 'execution errored',
  [TestResult_Status.PRECLUDED]: 'precluded',
});

export const TEST_STATUS_V2_CLASS_MAP = Object.freeze({
  [TestResult_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [TestResult_Status.FAILED]: 'failed-result',
  [TestResult_Status.PASSED]: 'passed-result',
  [TestResult_Status.SKIPPED]: 'skipped-result',
  [TestResult_Status.EXECUTION_ERRORED]: 'execution-errored-result',
  [TestResult_Status.PRECLUDED]: 'precluded-result',
});

export const WEB_TEST_STATUS_DISPLAY_MAP = Object.freeze({
  [WebTest_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [WebTest_Status.FAIL]: 'failed',
  [WebTest_Status.PASS]: 'passed',
  [WebTest_Status.SKIP]: 'skipped',
  [WebTest_Status.CRASH]: 'crashed',
  [WebTest_Status.TIMEOUT]: 'timed out',
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
