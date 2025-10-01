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

import { ReactNode } from 'react';

import { OutputSegment } from '@/analysis/types';

export interface StartPointInfoProps {
  readonly segment: OutputSegment;
  readonly instructionRow?: ReactNode;
}

export function StartPointInfo({
  segment,
  instructionRow,
}: StartPointInfoProps) {
  // Upper bound is inclusive, lower bound is exclusive.
  const commitCount =
    parseInt(segment.startPositionUpperBound99th) -
    parseInt(segment.startPositionLowerBound99th);
  // The 99th percentile lower bound is exclusive. Make it inclusive.
  const lowerBound99thInclusive = (
    parseInt(segment.startPositionLowerBound99th) + 1
  ).toString();
  return (
    <table>
      <thead>
        {instructionRow}
        <tr>
          <td colSpan={100}>
            The fix/culprit CL has 99% chance of occurring in:
          </td>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>Commits:</td>
          <td>
            {segment.startPositionUpperBound99th}...
            {lowerBound99thInclusive}~ ({commitCount} commits)
          </td>
        </tr>
      </tbody>
    </table>
  );
}
