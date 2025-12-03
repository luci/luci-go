// Copyright 2025 The LUCI Authors.
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

import { BuildCheckOptions } from '@/proto/turboci/data/build/v1/build_check_options.pb';
import { BuildCheckResult } from '@/proto/turboci/data/build/v1/build_check_results.pb';
import { GobSourceCheckOptions } from '@/proto/turboci/data/gerrit/v1/gob_source_check_options.pb';
import { PiperSourceCheckOptions } from '@/proto/turboci/data/piper/v1/piper_source_check_options.pb';
import { TestCheckDescriptionOption } from '@/proto/turboci/data/test/v1/test_check_description_option.pb';
import { TestCheckSummaryResult } from '@/proto/turboci/data/test/v1/test_check_summary_result.pb';
import { CheckKind } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { CheckView } from '@/proto/turboci/graph/orchestrator/v1/check_view.pb';
import { Datum } from '@/proto/turboci/graph/orchestrator/v1/datum.pb';

export enum CheckResultStatus {
  UNKNOWN = 'UNKNOWN',
  SUCCESS = 'SUCCESS',
  FAILURE = 'FAILURE',
}

export const TYPE_URL_BUILD_OPTIONS =
  'type.googleapis.com/turboci.data.build.v1.BuildCheckOptions';
export const TYPE_URL_BUILD_RESULT =
  'type.googleapis.com/turboci.data.build.v1.BuildCheckResult';
export const TYPE_URL_GOB_SOURCE_OPTIONS =
  'type.googleapis.com/turboci.data.gerrit.v1.GobSourceCheckOptions';
export const TYPE_URL_PIPER_SOURCE_OPTIONS =
  'type.googleapis.com/turboci.data.piper.v1.PiperSourceCheckOptions';
export const TYPE_URL_TEST_OPTIONS =
  'type.googleapis.com/turboci.data.test.v1.TestCheckDescriptionOption';
export const TYPE_URL_TEST_RESULT =
  'type.googleapis.com/turboci.data.test.v1.TestCheckSummaryResult';

/**
 * Safely parses the JSON content of a datum if the typeUrl matches.
 * Returns undefined if typeUrl mismatches, JSON is missing, or JSON is invalid.
 */
function parseDatum<T>(datum: Datum, expectedTypeUrl: string): T | undefined {
  const value = datum?.value;
  if (value?.value?.typeUrl !== expectedTypeUrl || !value?.valueJson) {
    return undefined;
  }
  try {
    return JSON.parse(value.valueJson);
  } catch {
    return undefined;
  }
}

export function getCheckResultStatus(checkView: CheckView): CheckResultStatus {
  const check = checkView.check;
  if (!check) return CheckResultStatus.UNKNOWN;

  for (const result of check.results) {
    for (const datum of result.data) {
      const buildCheckResult = parseDatum<BuildCheckResult>(
        datum,
        TYPE_URL_BUILD_RESULT,
      );
      if (buildCheckResult) {
        return buildCheckResult.success
          ? CheckResultStatus.SUCCESS
          : CheckResultStatus.FAILURE;
      }

      const testCheckResult = parseDatum<TestCheckSummaryResult>(
        datum,
        TYPE_URL_TEST_RESULT,
      );
      if (testCheckResult) {
        return testCheckResult.success
          ? CheckResultStatus.SUCCESS
          : CheckResultStatus.FAILURE;
      }
    }
  }

  return CheckResultStatus.UNKNOWN;
}

export function getCheckLabel(checkView: CheckView): string {
  const check = checkView.check;
  if (!check) return 'Unknown Check';

  for (const datum of check.options) {
    const buildOpts = parseDatum<BuildCheckOptions>(
      datum,
      TYPE_URL_BUILD_OPTIONS,
    );
    if (buildOpts?.target?.namespace && buildOpts.target.name) {
      return `Build ${buildOpts.target.namespace}:${buildOpts.target.name}`;
    }

    const testOpts = parseDatum<TestCheckDescriptionOption>(
      datum,
      TYPE_URL_TEST_OPTIONS,
    );
    if (testOpts?.title) {
      return `Test ${testOpts.title}`;
    }

    const gobOpts = parseDatum<GobSourceCheckOptions>(
      datum,
      TYPE_URL_GOB_SOURCE_OPTIONS,
    );
    if (gobOpts?.gerritChanges?.length) {
      const cl = gobOpts.gerritChanges[0];
      return `Source ${cl.hostname}/${cl.changeNumber}/${cl.patchset}`;
    }

    const piperOpts = parseDatum<PiperSourceCheckOptions>(
      datum,
      TYPE_URL_PIPER_SOURCE_OPTIONS,
    );
    if (piperOpts) {
      return `Source google3@${piperOpts.clNumber || 'HEAD'}`;
    }
  }

  // Fallback to generic kind-based label
  const id = check.identifier?.id || 'Unknown';
  switch (check.kind) {
    case CheckKind.CHECK_KIND_BUILD:
      return `Build Check: ${id}`;
    case CheckKind.CHECK_KIND_TEST:
      return `Test Check: ${id}`;
    case CheckKind.CHECK_KIND_SOURCE:
      return `Source Check: ${id}`;
    case CheckKind.CHECK_KIND_ANALYSIS:
      return `Analysis Check: ${id}`;
    default:
      return `Check: ${id}`;
  }
}
