// Copyright 2022 The LUCI Authors.
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
/* eslint-disable jest/no-conditional-expect */

import {
  impactFilterNamed,
  newMockFailure,
  newMockGroup,
} from '@/clusters/testing_tools/mocks/failures_mock';
import { DistinctClusterFailure } from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

import {
  FailureGroup,
  groupFailures,
  rejectedIngestedInvocationIdsExtractor,
  rejectedPresubmitRunIdsExtractor,
  sortFailureGroups,
  treeDistinctValues,
} from './failures_tools';

interface ExtractorTestCase {
  failure: DistinctClusterFailure;
  filter: string;
  shouldExtractIngestedInvocationId: boolean;
  shouldExtractPresubmitRunId: boolean;
}

describe.each<ExtractorTestCase>([
  {
    failure: newMockFailure().build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().exonerateNotCritical().build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().exonerateOccursOnOtherCLs().build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().exonerateNotCritical().build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().ingestedInvocationBlocked().build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().ingestedInvocationBlocked().buildFailed().build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .buildFailed()
      .notPresubmitCritical()
      .build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .buildFailed()
      .dryRun()
      .build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .buildFailed()
      .exonerateOccursOnOtherCLs()
      .build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .buildFailed()
      .exonerateNotCritical()
      .build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().exonerateOccursOnOtherCLs().build(),
    filter: 'None (Actual Impact)',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().ingestedInvocationBlocked().build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .notPresubmitCritical()
      .build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().ingestedInvocationBlocked().dryRun().build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().exonerateOccursOnOtherCLs().build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateOccursOnOtherCLs()
      .build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateOccursOnOtherCLs()
      .notPresubmitCritical()
      .build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().exonerateNotCritical().build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateNotCritical()
      .build(),
    filter: 'Without LUCI Analysis Exoneration',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().ingestedInvocationBlocked().build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure().exonerateOccursOnOtherCLs().build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateOccursOnOtherCLs()
      .build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateOccursOnOtherCLs()
      .notPresubmitCritical()
      .build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateOccursOnOtherCLs()
      .dryRun()
      .build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure().exonerateNotCritical().build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: false,
    shouldExtractPresubmitRunId: false,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateNotCritical()
      .build(),
    filter: 'Without All Exoneration',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure().build(),
    filter: 'Without Any Retries',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure().ingestedInvocationBlocked().build(),
    filter: 'Without Any Retries',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure().exonerateOccursOnOtherCLs().build(),
    filter: 'Without Any Retries',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateOccursOnOtherCLs()
      .build(),
    filter: 'Without Any Retries',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: true,
  },
  {
    failure: newMockFailure()
      .ingestedInvocationBlocked()
      .exonerateOccursOnOtherCLs()
      .notPresubmitCritical()
      .build(),
    filter: 'Without Any Retries',
    shouldExtractIngestedInvocationId: true,
    shouldExtractPresubmitRunId: false,
  },
])('Extractors with %j', (tc: ExtractorTestCase) => {
  it('should return ids in only the cases expected by failure type and impact filter.', () => {
    const ingestedInvocationIds = rejectedIngestedInvocationIdsExtractor(
      impactFilterNamed(tc.filter),
    )(tc.failure);
    if (tc.shouldExtractIngestedInvocationId) {
      expect(ingestedInvocationIds.size).toBeGreaterThan(0);
    } else {
      expect(ingestedInvocationIds.size).toBe(0);
    }
    const presubmitRunIds = rejectedPresubmitRunIdsExtractor(
      impactFilterNamed(tc.filter),
    )(tc.failure);
    if (tc.shouldExtractPresubmitRunId) {
      expect(presubmitRunIds.size).toBeGreaterThan(0);
    } else {
      expect(presubmitRunIds.size).toBe(0);
    }
  });
});

describe('groupFailures', () => {
  it('should put each failure in a separate group when given unique grouping keys', () => {
    const failures = [
      newMockFailure().build(),
      newMockFailure().build(),
      newMockFailure().build(),
    ];
    let unique = 0;
    const groups: FailureGroup[] = groupFailures(failures, () => [
      { type: 'variant', key: 'v1', value: '' + unique++ },
    ]);
    expect(groups.length).toBe(3);
    expect(groups[0].children.length).toBe(1);
  });
  it('should put each failure in a single group when given a single grouping key', () => {
    const failures = [
      newMockFailure().build(),
      newMockFailure().build(),
      newMockFailure().build(),
    ];
    const groups: FailureGroup[] = groupFailures(failures, () => [
      { type: 'variant', key: 'v1', value: 'group1' },
    ]);
    expect(groups.length).toBe(1);
    expect(groups[0].children.length).toBe(3);
  });
  it('should put group failures into multiple levels', () => {
    const failures = [
      newMockFailure()
        .withVariantGroups('v1', 'a')
        .withVariantGroups('v2', 'a')
        .build(),
      newMockFailure()
        .withVariantGroups('v1', 'a')
        .withVariantGroups('v2', 'b')
        .build(),
      newMockFailure()
        .withVariantGroups('v1', 'b')
        .withVariantGroups('v2', 'a')
        .build(),
      newMockFailure()
        .withVariantGroups('v1', 'b')
        .withVariantGroups('v2', 'b')
        .build(),
    ];
    const groups: FailureGroup[] = groupFailures(failures, (f) => [
      { type: 'variant', key: 'v1', value: f.variant?.def['v1'] || '' },
      { type: 'variant', key: 'v2', value: f.variant?.def['v2'] || '' },
    ]);
    expect(groups.length).toBe(2);
    expect(groups[0].children.length).toBe(2);
    expect(groups[0].commonVariant).toEqual({ def: { v1: 'a' } });
    expect(groups[1].children.length).toBe(2);
    expect(groups[1].commonVariant).toEqual({ def: { v1: 'b' } });
    expect(groups[0].children[0].children.length).toBe(1);
    expect(groups[0].children[0].commonVariant).toEqual({
      def: { v1: 'a', v2: 'a' },
    });
  });
});

describe('treeDistinctValues', () => {
  // A helper to just store the counts to the failures field.
  const setFailures = (g: FailureGroup, values: Set<string>) => {
    g.failures = values.size;
  };
  it('should have count of 1 for a valid feature', () => {
    const groups = groupFailures([newMockFailure().build()], () => [
      { type: 'variant', key: 'v1', value: 'group' },
    ]);

    treeDistinctValues(groups[0], () => new Set(['a']), setFailures);

    expect(groups[0].failures).toBe(1);
  });
  it('should have count of 0 for an invalid feature', () => {
    const groups = groupFailures([newMockFailure().build()], () => [
      { type: 'variant', key: 'v1', value: 'group' },
    ]);

    treeDistinctValues(groups[0], () => new Set(), setFailures);

    expect(groups[0].failures).toBe(0);
  });

  it('should have count of 1 for two identical features', () => {
    const groups = groupFailures(
      [newMockFailure().build(), newMockFailure().build()],
      () => [{ type: 'variant', key: 'v1', value: 'group' }],
    );

    treeDistinctValues(groups[0], () => new Set(['a']), setFailures);

    expect(groups[0].failures).toBe(1);
  });
  it('should have count of 2 for two different features', () => {
    const groups = groupFailures(
      [
        newMockFailure().withTestId('a').build(),
        newMockFailure().withTestId('b').build(),
      ],
      () => [{ type: 'variant', key: 'v1', value: 'group' }],
    );

    treeDistinctValues(
      groups[0],
      (f) => (f.testId ? new Set([f.testId]) : new Set()),
      setFailures,
    );

    expect(groups[0].failures).toBe(2);
  });
  it('should have count of 1 for two identical features in different subgroups', () => {
    const groups = groupFailures(
      [
        newMockFailure()
          .withTestId('a')
          .withVariantGroups('group', 'a')
          .build(),
        newMockFailure()
          .withTestId('a')
          .withVariantGroups('group', 'b')
          .build(),
      ],
      (f) => [
        { type: 'variant', key: 'v1', value: 'top' },
        { type: 'variant', key: 'v1', value: f.variant?.def['group'] || '' },
      ],
    );

    treeDistinctValues(
      groups[0],
      (f) => (f.testId ? new Set([f.testId]) : new Set()),
      setFailures,
    );

    expect(groups[0].failures).toBe(1);
    expect(groups[0].children[0].failures).toBe(1);
    expect(groups[0].children[1].failures).toBe(1);
  });
  it('should have count of 2 for two different features in different subgroups', () => {
    const groups = groupFailures(
      [
        newMockFailure()
          .withTestId('a')
          .withVariantGroups('group', 'a')
          .build(),
        newMockFailure()
          .withTestId('b')
          .withVariantGroups('group', 'b')
          .build(),
      ],
      (f) => [
        { type: 'variant', key: 'v1', value: 'top' },
        { type: 'variant', key: 'v1', value: f.variant?.def['group'] || '' },
      ],
    );

    treeDistinctValues(
      groups[0],
      (f) => (f.testId ? new Set([f.testId]) : new Set()),
      setFailures,
    );

    expect(groups[0].failures).toBe(2);
    expect(groups[0].children[0].failures).toBe(1);
    expect(groups[0].children[1].failures).toBe(1);
  });
});

describe('sortFailureGroups', () => {
  it('sorts top level groups ascending', () => {
    let groups: FailureGroup[] = [
      newMockGroup({ type: 'variant', key: 'v1', value: 'c' })
        .withFailures(3)
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'a' })
        .withFailures(1)
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'b' })
        .withFailures(2)
        .build(),
    ];

    groups = sortFailureGroups(groups, 'failures', true);

    expect(groups.map((g) => g.key.value)).toEqual(['a', 'b', 'c']);
  });
  it('sorts top level groups descending', () => {
    let groups: FailureGroup[] = [
      newMockGroup({ type: 'variant', key: 'v1', value: 'c' })
        .withFailures(3)
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'a' })
        .withFailures(1)
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'b' })
        .withFailures(2)
        .build(),
    ];

    groups = sortFailureGroups(groups, 'failures', false);

    expect(groups.map((g) => g.key.value)).toEqual(['c', 'b', 'a']);
  });
  it('sorts child groups', () => {
    let groups: FailureGroup[] = [
      newMockGroup({ type: 'variant', key: 'v1', value: 'c' })
        .withFailures(3)
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'a' })
        .withFailures(1)
        .withChildren([
          newMockGroup({ type: 'variant', key: 'v2', value: 'a3' })
            .withFailures(3)
            .build(),
          newMockGroup({ type: 'variant', key: 'v2', value: 'a2' })
            .withFailures(2)
            .build(),
          newMockGroup({ type: 'variant', key: 'v2', value: 'a1' })
            .withFailures(1)
            .build(),
        ])
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'b' })
        .withFailures(2)
        .build(),
    ];

    groups = sortFailureGroups(groups, 'failures', true);

    expect(groups.map((g) => g.key.value)).toEqual(['a', 'b', 'c']);
    expect(groups[0].children.map((g) => g.key.value)).toEqual([
      'a1',
      'a2',
      'a3',
    ]);
  });
  it('sorts on an alternate metric', () => {
    let groups: FailureGroup[] = [
      newMockGroup({ type: 'variant', key: 'v1', value: 'c' })
        .withPresubmitRejects(3)
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'a' })
        .withPresubmitRejects(1)
        .build(),
      newMockGroup({ type: 'variant', key: 'v1', value: 'b' })
        .withPresubmitRejects(2)
        .build(),
    ];

    groups = sortFailureGroups(groups, 'presubmitRejects', true);

    expect(groups.map((g) => g.key.value)).toEqual(['a', 'b', 'c']);
  });
});
