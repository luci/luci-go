// Copyright 2023 The LUCI Authors.
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
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { FailureReason_Kind } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/failure_reason.pb';
import {
  SkippedReason_Kind,
  TestResult,
  TestResult_Status,
  TestStatus,
  WebTest_Status,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

export const VERDICT_STATUS_OVERRIDE_DISPLAY_MAP = Object.freeze({
  [TestVerdict_StatusOverride.EXONERATED]: 'exonerated',
});

export const VERDICT_STATUS_DISPLAY_MAP = Object.freeze({
  [TestVerdict_Status.FAILED]: 'failed',
  [TestVerdict_Status.EXECUTION_ERRORED]: 'execution errored',
  [TestVerdict_Status.PRECLUDED]: 'precluded',
  [TestVerdict_Status.FLAKY]: 'flaky',
  [TestVerdict_Status.PASSED]: 'passed',
  [TestVerdict_Status.SKIPPED]: 'skipped',
});

export const TEST_STATUS_DISPLAY_MAP = Object.freeze({
  [TestStatus.STATUS_UNSPECIFIED]: 'unspecified',
  [TestStatus.PASS]: 'passed',
  [TestStatus.FAIL]: 'failed',
  [TestStatus.SKIP]: 'skipped',
  [TestStatus.CRASH]: 'crashed',
  [TestStatus.ABORT]: 'aborted',
});

export const TEST_STATUS_V2_DISPLAY_MAP = Object.freeze({
  [TestResult_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [TestResult_Status.PASSED]: 'passed',
  [TestResult_Status.FAILED]: 'failed',
  [TestResult_Status.SKIPPED]: 'skipped',
  [TestResult_Status.EXECUTION_ERRORED]: 'execution errored',
  [TestResult_Status.PRECLUDED]: 'precluded',
});

export const TEST_STATUS_V2_CLASS_MAP = Object.freeze({
  [TestResult_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [TestResult_Status.PASSED]: 'passed-result',
  [TestResult_Status.FAILED]: 'failed-result',
  [TestResult_Status.SKIPPED]: 'skipped-result',
  [TestResult_Status.EXECUTION_ERRORED]: 'execution-errored-result',
  [TestResult_Status.PRECLUDED]: 'precluded-result',
});

export const WEB_TEST_STATUS_DISPLAY_MAP = Object.freeze({
  [WebTest_Status.STATUS_UNSPECIFIED]: 'unspecified',
  [WebTest_Status.PASS]: 'passed',
  [WebTest_Status.FAIL]: 'failed',
  [WebTest_Status.SKIP]: 'skipped',
  [WebTest_Status.CRASH]: 'crashed',
  [WebTest_Status.TIMEOUT]: 'timed out',
});

// This list defines the order in which variant def keys should be displayed.
// Any unrecognized keys will be listed after the ones defined below.
export const ORDERED_VARIANT_DEF_KEYS = Object.freeze([
  'bucket',
  'builder',
  'test_suite',
]);

/* Returns the full status label for a test result. E.g. "skipped (demoted)" or "failed (unexpectedly passed)". */
export function testResultStatusLabel(tr: TestResult): string {
  const webTest = tr.frameworkExtensions?.webTest;
  const failureKind =
    tr.failureReason?.kind || FailureReason_Kind.KIND_UNSPECIFIED;
  const skippedKind =
    tr.skippedReason?.kind || SkippedReason_Kind.KIND_UNSPECIFIED;

  let statusDetail: string = '';
  if (webTest) {
    statusDetail = `${webTest.isExpected ? 'expectedly' : 'unexpectedly'} ${WEB_TEST_STATUS_DISPLAY_MAP[webTest.status]}`;
  } else if (
    tr.statusV2 === TestResult_Status.FAILED &&
    failureKind !== FailureReason_Kind.KIND_UNSPECIFIED
  ) {
    switch (failureKind) {
      case FailureReason_Kind.CRASH:
        statusDetail = 'crashed';
        break;
      case FailureReason_Kind.TIMEOUT:
        statusDetail = 'timed out';
        break;
      case FailureReason_Kind.ORDINARY:
        // No detail to show.
        break;
    }
  } else if (
    tr.statusV2 === TestResult_Status.SKIPPED &&
    skippedKind !== SkippedReason_Kind.KIND_UNSPECIFIED
  ) {
    switch (skippedKind) {
      case SkippedReason_Kind.DEMOTED:
        statusDetail = 'demoted';
        break;
      case SkippedReason_Kind.DISABLED_AT_DECLARATION:
        statusDetail = 'disabled at declaration';
        break;
      case SkippedReason_Kind.SKIPPED_BY_TEST_BODY:
        statusDetail = 'by test body';
        break;
      case SkippedReason_Kind.OTHER:
        // No status detail to show, but the failure reason section will contain information.
        break;
    }
  }
  if (statusDetail) {
    return `${TEST_STATUS_V2_DISPLAY_MAP[tr.statusV2]} (${statusDetail})`;
  }
  return TEST_STATUS_V2_DISPLAY_MAP[tr.statusV2];
}

export const ARTIFACT_LENGTH_LIMIT = 50000;
