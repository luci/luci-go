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
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { ValueRef } from '@/proto/turboci/graph/orchestrator/v1/value_ref.pb';

import { INVALID_IDENTIFIER, toString as idToString } from './id';
import {
  LegacyWorkNode,
  TYPE_URL_LEGACY_WORKNODE_STAGE,
  extractLegacyWorkNodeLabel,
} from './legacy_worknode';

export enum CheckResultStatus {
  UNKNOWN = 'UNKNOWN',
  SUCCESS = 'SUCCESS',
  FAILURE = 'FAILURE',
  MIXED = 'MIXED',
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

export function getStageLabel(
  stage: Stage,
  valueDataMap: Map<string, ValueData>,
): string {
  if (!stage) return 'Unknown Stage';
  const id = stage.identifier?.id || 'Unknown';

  if (stage.args?.typeUrl === TYPE_URL_LEGACY_WORKNODE_STAGE) {
    const legacyData = parseValueRef<LegacyWorkNode>(
      stage.args,
      TYPE_URL_LEGACY_WORKNODE_STAGE,
      valueDataMap,
    );
    const label = extractLegacyWorkNodeLabel(legacyData, id);
    if (label) return label;
  }

  return `Stage: ${id}`;
}

function isStage(view: Check | Stage): view is Stage {
  return (view as Stage).assignments !== undefined;
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
