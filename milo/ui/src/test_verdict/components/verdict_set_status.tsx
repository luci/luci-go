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

import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { VERDICT_STATUS_COLOR_MAP } from '@/test_verdict/constants/verdict';

import { VerdictStatusIcon } from './verdict_status_icon';

const STATUSES = Object.freeze([
  TestVariantStatus.UNEXPECTED,
  TestVariantStatus.UNEXPECTEDLY_SKIPPED,
  TestVariantStatus.FLAKY,
  TestVariantStatus.EXONERATED,
  TestVariantStatus.EXPECTED,
] as const);

export interface VerdictCounts {
  readonly [TestVariantStatus.UNEXPECTED]?: number;
  readonly [TestVariantStatus.UNEXPECTEDLY_SKIPPED]?: number;
  readonly [TestVariantStatus.FLAKY]?: number;
  readonly [TestVariantStatus.EXONERATED]?: number;
  readonly [TestVariantStatus.EXPECTED]?: number;
}

export interface VerdictSetStatusProps {
  readonly counts: VerdictCounts;
}

/**
 * VerdictSetStatus display statuses for a list of verdicts.
 */
export function VerdictSetStatus({ counts }: VerdictSetStatusProps) {
  const total = STATUSES.reduce((prev, s) => prev + (counts[s] || 0), 0);
  if (total === 0) {
    return <></>;
  }

  const singleStatus = STATUSES.find((s) => counts[s] === total);
  if (singleStatus !== undefined) {
    return (
      <VerdictStatusIcon
        status={singleStatus}
        sx={{ verticalAlign: 'middle' }}
      />
    );
  }
  return <TestVerdictsPieChart counts={counts} />;
}

interface VerdictGroup {
  readonly color: string;
  readonly count: number;
}

const pieGenerator = pie<unknown, VerdictGroup>()
  .value((d) => d.count)
  .sort(null);
const arcPathGenerator = arc();

interface TestVerdictsPieChartProps {
  readonly counts: VerdictCounts;
}

function TestVerdictsPieChart({ counts }: TestVerdictsPieChartProps) {
  const slices = useMemo(() => {
    const groups = [];
    for (const s of STATUSES) {
      const count = counts[s];
      if (!count) {
        continue;
      }
      groups.push({
        color: VERDICT_STATUS_COLOR_MAP[s],
        count,
      });
    }
    return pieGenerator(groups).map((p) => ({
      arc: arcPathGenerator({
        innerRadius: 4,
        outerRadius: 9,
        startAngle: p.startAngle,
        endAngle: p.endAngle,
      })!,
      color: p.data.color,
    }));
  }, [counts]);

  if (slices.length === 0) {
    return <></>;
  }

  return (
    <svg
      viewBox="-10 -10 20 20"
      width="24px"
      height="24px"
      css={{ verticalAlign: 'middle' }}
    >
      {slices.map((slice, i) => (
        <path key={i} d={slice.arc} fill={slice.color} />
      ))}
    </svg>
  );
}
