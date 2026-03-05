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
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { ValueRef } from '@/proto/turboci/graph/orchestrator/v1/value_ref.pb';

import {
  CheckResultStatus,
  getCheckLabel,
  getCheckResultStatus,
  TYPE_URL_BUILD_OPTIONS,
  TYPE_URL_BUILD_RESULT,
  TYPE_URL_GOB_SOURCE_OPTIONS,
  TYPE_URL_PIPER_SOURCE_OPTIONS,
  TYPE_URL_TEST_OPTIONS,
  TYPE_URL_TEST_RESULT,
} from './check_utils';

function createCheck(
  kind: CheckKind,
  id: string,
  valueDataMap: Map<string, ValueData>,
  optionsData: { typeUrl: string; digest: string; json: unknown }[] = [],
  resultsData: { typeUrl: string; digest: string; json: unknown }[] = [],
): Check {
  const options: ValueRef[] = optionsData.map((o) => ({
    realm: 'test-realm',
    typeUrl: o.typeUrl,
    digest: o.digest,
  }));
  optionsData.forEach((o) => {
    valueDataMap.set(o.digest, {
      json: { value: o.json !== undefined ? JSON.stringify(o.json) : '' },
    });
  });

  const results =
    resultsData.length > 0
      ? [
          {
            identifier: { check: { id, workPlan: { id: 'test-wp' } }, idx: 1 },
            owner: { orchestrator: {} },
            data: resultsData.map((r) => ({
              realm: 'test-realm',
              typeUrl: r.typeUrl,
              digest: r.digest,
            })),
            createdAt: { ts: '2024-01-01T12:00:00Z' },
            finalizedAt: { ts: '2024-01-01T12:05:00Z' },
          },
        ]
      : [];
  resultsData.forEach((r) => {
    valueDataMap.set(r.digest, {
      json: { value: r.json !== undefined ? JSON.stringify(r.json) : '' },
    });
  });

  return {
    identifier: { id, workPlan: { id: 'test-wp' } },
    kind,
    realm: 'test-realm',
    version: { ts: '2024-01-01T12:05:00Z' },
    options,
    results,
    state: 0, // State should be set
    stateHistory: [],
    edits: [],
  } as Check;
}

describe('check_utils', () => {
  describe('getCheckResultStatus', () => {
    type TestCase = {
      name: string;
      kind: CheckKind;
      results: { typeUrl: string; digest: string; json: unknown }[];
      valueDataMap: Map<string, ValueData>;
      expected: CheckResultStatus;
    };
    const DIGEST = 'test-digest';

    const testCases: TestCase[] = [
      {
        name: 'returns UNKNOWN if no results present',
        kind: CheckKind.CHECK_KIND_BUILD,
        results: [],
        valueDataMap: new Map(),
        expected: CheckResultStatus.UNKNOWN,
      },
      {
        name: 'returns UNKNOWN if result type is unknown',
        kind: CheckKind.CHECK_KIND_BUILD,
        results: [
          { typeUrl: 'unknown.Type', digest: DIGEST, json: { success: true } },
        ],
        valueDataMap: new Map(),
        expected: CheckResultStatus.UNKNOWN,
      },
      {
        name: 'returns SUCCESS for BuildCheckResult with success=true',
        kind: CheckKind.CHECK_KIND_BUILD,
        results: [
          {
            typeUrl: TYPE_URL_BUILD_RESULT,
            digest: DIGEST,
            json: { success: true } as BuildCheckResult,
          },
        ],
        valueDataMap: new Map(),
        expected: CheckResultStatus.SUCCESS,
      },
      {
        name: 'returns FAILURE for BuildCheckResult with success=false',
        kind: CheckKind.CHECK_KIND_BUILD,
        results: [
          {
            typeUrl: TYPE_URL_BUILD_RESULT,
            digest: DIGEST,
            json: { success: false } as BuildCheckResult,
          },
        ],
        valueDataMap: new Map(),
        expected: CheckResultStatus.FAILURE,
      },
      {
        name: 'returns SUCCESS for TestCheckSummaryResult with success=true',
        kind: CheckKind.CHECK_KIND_TEST,
        results: [
          {
            typeUrl: TYPE_URL_TEST_RESULT,
            digest: DIGEST,
            json: { success: true } as TestCheckSummaryResult,
          },
        ],
        valueDataMap: new Map(),
        expected: CheckResultStatus.SUCCESS,
      },
      {
        name: 'returns FAILURE for TestCheckSummaryResult with success=false',
        kind: CheckKind.CHECK_KIND_TEST,
        results: [
          {
            typeUrl: TYPE_URL_TEST_RESULT,
            digest: DIGEST,
            json: { success: false } as TestCheckSummaryResult,
          },
        ],
        valueDataMap: new Map(),
        expected: CheckResultStatus.FAILURE,
      },
      {
        name: 'ignores results without JSON data',
        kind: CheckKind.CHECK_KIND_BUILD,
        results: [
          {
            typeUrl: TYPE_URL_BUILD_RESULT,
            digest: DIGEST,
            json: undefined,
          },
        ],
        valueDataMap: new Map(),
        expected: CheckResultStatus.UNKNOWN,
      },
    ];

    testCases.forEach((tc) => {
      it(`${tc.name}`, () => {
        const view = createCheck(
          tc.kind,
          'check-id',
          tc.valueDataMap,
          [],
          tc.results,
        );
        expect(getCheckResultStatus(view, tc.valueDataMap)).toBe(tc.expected);
      });
    });
  });

  describe('getCheckLabel', () => {
    type TestCase = {
      name: string;
      kind: CheckKind;
      id: string;
      options: { typeUrl: string; digest: string; json: unknown }[];
      valueDataMap: Map<string, ValueData>;
      expected: string;
    };
    const DIGEST = 'test-digest';

    const testCases: TestCase[] = [
      {
        name: 'Build: uses namespace and name',
        kind: CheckKind.CHECK_KIND_BUILD,
        id: 'C1',
        options: [
          {
            typeUrl: TYPE_URL_BUILD_OPTIONS,
            digest: DIGEST,
            json: {
              target: { namespace: 'ci', name: 'linux-rel' },
            } as BuildCheckOptions,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Build ci:linux-rel',
      },
      {
        name: 'Build: falls back to ID if target options missing',
        kind: CheckKind.CHECK_KIND_BUILD,
        id: 'C3',
        options: [
          {
            typeUrl: TYPE_URL_BUILD_OPTIONS,
            digest: DIGEST,
            json: {} as BuildCheckOptions,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Build Check: C3',
      },
      {
        name: 'Test: uses title',
        kind: CheckKind.CHECK_KIND_TEST,
        id: 'T1',
        options: [
          {
            typeUrl: TYPE_URL_TEST_OPTIONS,
            digest: DIGEST,
            json: {
              title: 'Unit Tests',
            } as TestCheckDescriptionOption,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Test Unit Tests',
      },
      {
        name: 'Test: falls back to ID if title missing',
        kind: CheckKind.CHECK_KIND_TEST,
        id: 'T2',
        options: [
          {
            typeUrl: TYPE_URL_TEST_OPTIONS,
            digest: DIGEST,
            json: {} as TestCheckDescriptionOption,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Test Check: T2',
      },
      {
        name: 'GoB Source: uses first gerrit change',
        kind: CheckKind.CHECK_KIND_SOURCE,
        id: 'S1',
        options: [
          {
            typeUrl: TYPE_URL_GOB_SOURCE_OPTIONS,
            digest: DIGEST,
            json: {
              gerritChanges: [
                {
                  hostname: 'chromium',
                  changeNumber: '123456',
                  patchset: 1,
                  mountsToApply: [],
                },
              ],
            } as GobSourceCheckOptions,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Source chromium/123456/1',
      },
      {
        name: 'GoB Source: falls back to ID if no changes',
        kind: CheckKind.CHECK_KIND_SOURCE,
        id: 'S2',
        options: [
          {
            typeUrl: TYPE_URL_GOB_SOURCE_OPTIONS,
            digest: DIGEST,
            json: { gerritChanges: [] } as GobSourceCheckOptions,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Source Check: S2',
      },
      {
        name: 'Piper Source: uses CL number',
        kind: CheckKind.CHECK_KIND_SOURCE,
        id: 'P1',
        options: [
          {
            typeUrl: TYPE_URL_PIPER_SOURCE_OPTIONS,
            digest: DIGEST,
            json: { clNumber: '987654321' } as PiperSourceCheckOptions,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Source google3@987654321',
      },
      {
        name: 'Piper Source: uses HEAD if CL number missing',
        kind: CheckKind.CHECK_KIND_SOURCE,
        id: 'P2',
        options: [
          {
            typeUrl: TYPE_URL_PIPER_SOURCE_OPTIONS,
            digest: DIGEST,
            json: {} as PiperSourceCheckOptions,
          },
        ],
        valueDataMap: new Map(),
        expected: 'Source google3@HEAD',
      },
      {
        name: 'Fallback: unknown kind uses generic label with ID',
        kind: CheckKind.CHECK_KIND_UNKNOWN,
        id: 'U1',
        options: [],
        valueDataMap: new Map(),
        expected: 'Check: U1',
      },
    ];

    testCases.forEach((tc) => {
      it(`${tc.name}`, () => {
        const view = createCheck(tc.kind, tc.id, tc.valueDataMap, tc.options);
        expect(getCheckLabel(view, tc.valueDataMap)).toBe(tc.expected);
      });
    });
  });
});
