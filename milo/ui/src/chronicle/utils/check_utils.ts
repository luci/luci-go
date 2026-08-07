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
import { Check } from '@/proto/turboci/graph/orchestrator/v1/check.pb';
import { CheckKind } from '@/proto/turboci/graph/orchestrator/v1/check_kind.pb';
import { Stage } from '@/proto/turboci/graph/orchestrator/v1/stage.pb';
import { StageAttemptState } from '@/proto/turboci/graph/orchestrator/v1/stage_attempt_state.pb';
import { StageConcludedReason } from '@/proto/turboci/graph/orchestrator/v1/stage_concluded_reason.pb';
import { StageState } from '@/proto/turboci/graph/orchestrator/v1/stage_state.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { ValueRef } from '@/proto/turboci/graph/orchestrator/v1/value_ref.pb';

import { INVALID_IDENTIFIER, toString as idToString } from './id';
import {
  extractLegacyWorkNode,
  extractLegacyWorkNodeLabel,
  TYPE_URL_LEGACY_WORKNODE_STAGE,
} from './legacy_worknode';

export enum CheckResultStatus {
  UNKNOWN = 'UNKNOWN',
  SUCCESS = 'SUCCESS',
  FAILURE = 'FAILURE',
  MIXED = 'MIXED',
}

export enum StageResultStatus {
  UNKNOWN = 'UNKNOWN',
  SUCCESS = 'SUCCESS',
  FAILURE = 'FAILURE',
  RUNNING = 'RUNNING',
  CANCELLED = 'CANCELLED',
  PENDING = 'PENDING',
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
 * Safely parses the JSON content of a ValueRef if the typeUrl matches.
 * Returns undefined if typeUrl mismatches, JSON is missing, or JSON is invalid.
 */
function parseValueRef<T>(
  value_ref: ValueRef,
  expectedTypeUrl: string,
  valueDataMap: Map<string, ValueData>,
): T | undefined {
  if (!value_ref.digest || value_ref.typeUrl !== expectedTypeUrl) {
    return undefined;
  }
  const valueData = valueDataMap.get(value_ref.digest);
  if (!valueData || !valueData.json || !valueData.json.value) {
    return undefined;
  }
  try {
    return JSON.parse(valueData.json.value);
  } catch {
    return undefined;
  }
}

export function getCheckResultStatus(
  check: Check,
  valueDataMap: Map<string, ValueData>,
): CheckResultStatus {
  if (!check) return CheckResultStatus.UNKNOWN;

  for (const result of check.results) {
    for (const value_ref of result.data) {
      const buildCheckResult = parseValueRef<BuildCheckResult>(
        value_ref,
        TYPE_URL_BUILD_RESULT,
        valueDataMap,
      );
      if (buildCheckResult) {
        return buildCheckResult.success
          ? CheckResultStatus.SUCCESS
          : CheckResultStatus.FAILURE;
      }

      const testCheckResult = parseValueRef<TestCheckSummaryResult>(
        value_ref,
        TYPE_URL_TEST_RESULT,
        valueDataMap,
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

export function getCheckLabel(
  check: Check,
  valueDataMap: Map<string, ValueData>,
): string {
  if (!check) return 'Unknown Check';

  for (const value_ref of check.options) {
    const buildOpts = parseValueRef<BuildCheckOptions>(
      value_ref,
      TYPE_URL_BUILD_OPTIONS,
      valueDataMap,
    );
    if (buildOpts?.target?.namespace && buildOpts.target.name) {
      return `Build ${buildOpts.target.namespace}:${buildOpts.target.name}`;
    }

    const testOpts = parseValueRef<TestCheckDescriptionOption>(
      value_ref,
      TYPE_URL_TEST_OPTIONS,
      valueDataMap,
    );
    if (testOpts?.title) {
      return `Test ${testOpts.title}`;
    }

    const gobOpts = parseValueRef<GobSourceCheckOptions>(
      value_ref,
      TYPE_URL_GOB_SOURCE_OPTIONS,
      valueDataMap,
    );
    if (gobOpts?.gerritChanges?.length) {
      const cl = gobOpts.gerritChanges[0];
      return `Source ${cl.hostname}/${cl.changeNumber}/${cl.patchset}`;
    }

    const piperOpts = parseValueRef<PiperSourceCheckOptions>(
      value_ref,
      TYPE_URL_PIPER_SOURCE_OPTIONS,
      valueDataMap,
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

export function isWorknodeStage(stage: Stage): boolean {
  if (!stage) return false;
  if (stage.identifier?.isWorknode !== undefined) {
    return stage.identifier.isWorknode;
  }
  if (stage.args?.typeUrl === TYPE_URL_LEGACY_WORKNODE_STAGE) {
    return true;
  }
  return false;
}

/**
 * Determines the visual result status (SUCCESS, FAILURE, RUNNING, CANCELLED, PENDING)
 * for a Stage node.
 *
 * Status resolution flow:
 * 1. PLANNED stages map to PENDING.
 * 2. ATTEMPTING stages map to PENDING or CANCELLED if their latest attempt matches
 *    those states, otherwise RUNNING.
 * 3. FINAL stages:
 *    a. Mapped to CANCELLED if concludedReason indicates cancellation.
 *    b. Mapped to FAILURE if all attempts finished without running to completion
 *       (e.g. NO_RETRIES_LEFT, TIMEOUT, FINAL_ATTEMPT_BLOCKED_RETRY, or latest attempt INCOMPLETE).
 *    c. Otherwise, an attempt ran to completion:
 *       - For N-stages (legacy WorkNodes), status is evaluated from `workOutput.success`
 *         (when available via valueDataMap).
 *       - For S-stages (native TurboCI stages), successful attempt completion maps to SUCCESS
 *         (with specific test/build verdicts residing on attached Checks).
 */
export function getStageResultStatus(
  stage: Stage,
  valueDataMap?: Map<string, ValueData>,
): StageResultStatus {
  if (!stage) return StageResultStatus.UNKNOWN;

  switch (stage.state) {
    case StageState.STAGE_STATE_PLANNED:
      return StageResultStatus.PENDING;
    case StageState.STAGE_STATE_ATTEMPTING: {
      const latestAttempt =
        stage.attempts && stage.attempts.length > 0
          ? stage.attempts[stage.attempts.length - 1]
          : undefined;
      switch (latestAttempt?.state) {
        case StageAttemptState.STAGE_ATTEMPT_STATE_PENDING:
          return StageResultStatus.PENDING;
        case StageAttemptState.STAGE_ATTEMPT_STATE_CANCELLING:
          return StageResultStatus.CANCELLED;
        default:
          return StageResultStatus.RUNNING;
      }
    }

    case StageState.STAGE_STATE_FINAL: {
      // 1. Check for cancellation
      if (
        stage.concludedReason ===
        StageConcludedReason.STAGE_CONCLUDED_REASON_CANCELLED
      ) {
        return StageResultStatus.CANCELLED;
      }

      // 2. Check if all attempts finished without running to completion (infra / orchestrator failures)
      const latestAttempt =
        stage.attempts && stage.attempts.length > 0
          ? stage.attempts[stage.attempts.length - 1]
          : undefined;
      const isAttemptIncomplete =
        latestAttempt?.state ===
        StageAttemptState.STAGE_ATTEMPT_STATE_INCOMPLETE;

      if (
        stage.concludedReason ===
          StageConcludedReason.STAGE_CONCLUDED_REASON_NO_RETRIES_LEFT ||
        stage.concludedReason ===
          StageConcludedReason.STAGE_CONCLUDED_REASON_FINAL_ATTEMPT_BLOCKED_RETRY ||
        stage.concludedReason ===
          StageConcludedReason.STAGE_CONCLUDED_REASON_TIMEOUT ||
        isAttemptIncomplete
      ) {
        return StageResultStatus.FAILURE;
      }

      // 3. The stage ran to completion:
      // Note: We currently merge non-completion and task failure into FAILURE.
      // For N-stages, the task outcome is embedded in `workOutput.success`. For
      // S-stages, the task outcome lives on attached Checks, while the Stage itself
      // succeeds if its attempt completed.
      if (valueDataMap) {
        const legacyWorkNode = extractLegacyWorkNode(stage, valueDataMap);
        if (legacyWorkNode?.workOutput?.success === false) {
          return StageResultStatus.FAILURE;
        } else if (legacyWorkNode?.workOutput?.success === true) {
          return StageResultStatus.SUCCESS;
        }
      }

      // For S-stages (and N-stages without explicit work output failures), map to SUCCESS
      if (
        stage.concludedReason ===
        StageConcludedReason.STAGE_CONCLUDED_REASON_ATTEMPT_COMPLETE
      ) {
        return StageResultStatus.SUCCESS;
      }

      return StageResultStatus.UNKNOWN;
    }
    default:
      return StageResultStatus.UNKNOWN;
  }
}

export function getStageLabel(
  stage: Stage,
  valueDataMap: Map<string, ValueData>,
): string {
  if (!stage) return 'Unknown Stage';
  const id = stage.identifier?.id || 'Unknown';

  if (isWorknodeStage(stage)) {
    const legacyData = extractLegacyWorkNode(stage, valueDataMap);
    const label = extractLegacyWorkNodeLabel(legacyData, id);
    if (label) return label;
  }

  return `Stage: ${id}`;
}

export function isStage(view?: Check | Stage): view is Stage {
  return (view as Stage)?.assignments !== undefined;
}

/**
 * Prepares a Check or Stage object for search index serialization by creating a shallow copy
 * with fields that could lead to false-positive matches removed.
 */
function createIndexableObject(view: Check | Stage): Partial<Check | Stage> {
  const obj = {
    ...view,
    // Exclude dependencies so we don't match on dependency IDs
    dependencies: undefined,
  };
  if (isStage(view)) {
    return {
      ...obj,
      // Exclude assignments so we don't match on assigned check IDs.
      assignments: undefined,
    };
  }
  return obj;
}

/**
 * Builds a normalized, lowercase full-text search string for a node by combining its
 * ID, canonical identifier format, label, and serialized metadata, while excluding
 * connection fields (assignments and dependencies) to prevent false-positive matches.
 */
export function getNodeSearchIndex(
  id: string,
  label: string,
  view?: Check | Stage,
): string {
  const parts: string[] = [id, label];
  if (view) {
    if (view.identifier) {
      const canonicalId = idToString(view.identifier);
      if (canonicalId !== INVALID_IDENTIFIER) {
        parts.push(canonicalId);
      }
    }
    const objToSerialize = createIndexableObject(view);
    parts.push(JSON.stringify(objToSerialize));
  }
  return parts.join(' ').toLowerCase();
}
