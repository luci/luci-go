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

import { DateTime } from 'luxon';
import { ReactNode } from 'react';

import { OutputSegment } from '@/analysis/types';
import { VerdictStatusIcon } from '@/common/components/verdict_status_icon';
import { VERDICT_STATUS_COLOR_MAP } from '@/common/constants/verdict';
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { SpecifiedTestVerdictStatus } from '@/common/types/verdict';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

interface SegmentStatRowProps {
  readonly status: SpecifiedTestVerdictStatus;
  readonly label: string;
  readonly count: number;
  readonly total: number;
}

function SegmentStatRow({ status, label, count, total }: SegmentStatRowProps) {
  const percentText =
    total === 0
      ? ''
      : ` (${(count / total).toLocaleString(undefined, {
          style: 'percent',
          maximumFractionDigits: 1,
          minimumFractionDigits: 0,
        })})`;

  return (
    <tr>
      <td>
        <VerdictStatusIcon statusV2={status} sx={{ verticalAlign: 'middle' }} />{' '}
        {label}:
      </td>
      <td
        css={{
          color:
            count > 0
              ? VERDICT_STATUS_COLOR_MAP[status]
              : 'var(--greyed-out-text-color)',
        }}
      >
        {count}
        {percentText}
      </td>
    </tr>
  );
}

export interface SegmentInfoProps {
  readonly segment: OutputSegment;
  readonly instructionRow?: ReactNode;
}

export function SegmentInfo({ segment, instructionRow }: SegmentInfoProps) {
  const startHour = DateTime.fromISO(segment.startHour);
  const endHour = DateTime.fromISO(segment.endHour);

  const expectedVerdictCount =
    segment.counts.totalVerdicts -
    segment.counts.unexpectedVerdicts -
    segment.counts.flakyVerdicts;

  const expectedResults =
    segment.counts.totalResults - segment.counts.unexpectedResults;
  const commitCount =
    parseInt(segment.endPosition) - parseInt(segment.startPosition) + 1;

  return (
    <table>
      {instructionRow && <thead>{instructionRow}</thead>}
      <tbody>
        <tr>
          <td>Commits:</td>
          <td>
            {segment.endPosition}...
            {segment.startPosition}~ ({commitCount} commits)
          </td>
        </tr>
        <tr>
          <td>Time Range:</td>
          <td>
            {startHour.toFormat(SHORT_TIME_FORMAT)} ~{' '}
            {endHour.toFormat(SHORT_TIME_FORMAT)}
          </td>
        </tr>
        <SegmentStatRow
          status={TestVerdict_Status.FAILED}
          label="Failed Source Verdicts"
          count={segment.counts.unexpectedVerdicts}
          total={segment.counts.totalVerdicts}
        />
        <SegmentStatRow
          status={TestVerdict_Status.FLAKY}
          label="Flaky Source Verdicts"
          count={segment.counts.flakyVerdicts}
          total={segment.counts.totalVerdicts}
        />
        <SegmentStatRow
          status={TestVerdict_Status.PASSED}
          label="Passed Source Verdicts"
          count={expectedVerdictCount}
          total={segment.counts.totalVerdicts}
        />
        <SegmentStatRow
          status={TestVerdict_Status.FAILED}
          label="Failed Test Results"
          count={segment.counts.unexpectedResults}
          total={segment.counts.totalResults}
        />
        <SegmentStatRow
          status={TestVerdict_Status.PASSED}
          label="Passed Test Results"
          count={expectedResults}
          total={segment.counts.totalResults}
        />
      </tbody>
    </table>
  );
}
