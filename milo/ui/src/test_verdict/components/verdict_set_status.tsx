// Copyright 2024 The LUCI Authors.
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

import { arc, pie } from 'd3';
import { useMemo } from 'react';

import {
  OutputTestVerdict,
  SpecifiedTestVerdictStatus as AnalysisVerdictStatus,
} from '@/analysis/types';
import { TestVerdictStatus } from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_verdict.pb';
import { VERDICT_STATUS_COLOR_MAP } from '@/test_verdict/constants/verdict';

import { SpecifiedTestVerdictStatus } from '../types';

import { VerdictStatusIcon } from './verdict_status_icon';

export interface VerdictSetStatusProps {
  readonly testVerdicts: readonly OutputTestVerdict[];
}

/**
 * VerdictSetStatus display statuses when there's a list of verdicts.
 */
export function VerdictSetStatus({ testVerdicts }: VerdictSetStatusProps) {
  if (testVerdicts.length === 0) {
    return <></>;
  }

  const allStatuses = new Set(testVerdicts.map((tv) => tv.status));
  if (allStatuses.size === 1) {
    return (
      <VerdictStatusIcon
        status={SpecifiedTestVerdictStatus.fromAnalysis(testVerdicts[0].status)}
        sx={{ verticalAlign: 'middle' }}
      />
    );
  }
  return <TestVerdictsPieChart testVerdicts={testVerdicts} />;
}

interface VerdictGroup {
  readonly color: string;
  readonly count: number;
}

const pieGenerator = pie<unknown, VerdictGroup>()
  .value((d) => d.count)
  .sort(null);
const arcPathGenerator = arc();

const status: AnalysisVerdictStatus[] = [
  TestVerdictStatus.EXPECTED,
  TestVerdictStatus.UNEXPECTED,
  TestVerdictStatus.EXONERATED,
  TestVerdictStatus.FLAKY,
  TestVerdictStatus.UNEXPECTEDLY_SKIPPED,
];

export interface TestVerdictsPieChartProps {
  readonly testVerdicts: readonly OutputTestVerdict[];
}

export function TestVerdictsPieChart({
  testVerdicts,
}: TestVerdictsPieChartProps) {
  const { groups, arcs } = useMemo(() => {
    const groups = [];
    for (const s of status) {
      const count = testVerdicts.reduce(
        (c, tv) => (tv.status === s ? c + 1 : c),
        0,
      );
      groups.push({
        color: VERDICT_STATUS_COLOR_MAP[s],
        count,
      });
    }
    const arcs = pieGenerator(groups).map((p) =>
      arcPathGenerator({
        innerRadius: 4,
        outerRadius: 9,
        startAngle: p.startAngle,
        endAngle: p.endAngle,
      }),
    );
    return { groups, arcs };
  }, [testVerdicts]);
  if (!testVerdicts || testVerdicts.length === 0) {
    return <></>;
  }
  return (
    <svg
      viewBox="-10 -10 20 20"
      width="24px"
      height="24px"
      css={{ transform: 'translateY(2px)' }}
    >
      <g>
        {arcs.map((arc, i) => (
          <path key={i} d={arc!} fill={groups[i].color} />
        ))}
      </g>
    </svg>
  );
}
