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

import { Box } from '@mui/material';
import { arc, pie } from 'd3';
import { useMemo } from 'react';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { ChangepointGroupStatistics_RateDistribution_RateBuckets } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';

interface FailureRateGroup {
  readonly label: string;
  readonly color: string;
  readonly count: number;
}

const pieGenerator = pie<unknown, FailureRateGroup>()
  .value((d) => d.count)
  .sort(null);
const arcPathGenerator = arc();

export interface FailureRatePieChartProps {
  readonly label: string;
  readonly buckets: ChangepointGroupStatistics_RateDistribution_RateBuckets;
}

export function FailureRatePieChart({
  label,
  buckets,
}: FailureRatePieChartProps) {
  const { groups, arcs } = useMemo(() => {
    const groups = [
      {
        label: '≥95%',
        color: 'var(--failure-color)',
        count: buckets.countAbove95Percent,
      },
      {
        label: '5~95%',
        color: 'var(--warning-color)',
        count: buckets.countAbove5LessThan95Percent,
      },
      {
        label: '<5%',
        color: 'var(--success-color)',
        count: buckets.countLess5Percent,
      },
    ];
    const arcs = pieGenerator(groups).map((p) =>
      arcPathGenerator({
        innerRadius: 4,
        outerRadius: 9,
        startAngle: p.startAngle,
        endAngle: p.endAngle,
      }),
    );
    return { groups, arcs };
  }, [buckets]);

  return (
    <HtmlTooltip
      arrow
      title={
        <Box sx={{ width: '300px' }}>
          <b>{label}:</b>
          <table>
            <thead>
              <tr>
                <th align="left">Failure Rate</th>
                <th>Count</th>
              </tr>
            </thead>
            <tbody>
              {groups.map((g, i) => (
                <tr key={i} css={{ color: g.color }}>
                  <td width="100px">
                    <b>{g.label}</b>
                  </td>
                  <td>{g.count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Box>
      }
    >
      <svg
        viewBox="-10 -10 20 20"
        width="35px"
        height="35px"
        css={{ transform: 'translateY(2px)' }}
      >
        <g>
          {arcs.map((arc, i) => (
            <path key={i} d={arc!} fill={groups[i].color} />
          ))}
        </g>
      </svg>
    </HtmlTooltip>
  );
}
