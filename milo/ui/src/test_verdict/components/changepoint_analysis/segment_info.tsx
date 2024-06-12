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
import { SHORT_TIME_FORMAT } from '@/common/tools/time_utils';
import { TestVariantStatus } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';
import { VerdictStatusIcon } from '@/test_verdict/components/verdict_status_icon';
import { VERDICT_STATUS_COLOR_MAP } from '@/test_verdict/constants/verdict';

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
        <tr>
          <td>
            <VerdictStatusIcon
              status={TestVariantStatus.UNEXPECTED}
              sx={{ verticalAlign: 'middle' }}
            />{' '}
            Unexpected Source Verdicts:
          </td>
          <td
            css={{
              color:
                segment.counts.unexpectedVerdicts > 0
                  ? VERDICT_STATUS_COLOR_MAP[TestVariantStatus.UNEXPECTED]
                  : 'var(--greyed-out-text-color)',
            }}
          >
            {segment.counts.unexpectedVerdicts}
          </td>
        </tr>
        <tr>
          <td>
            <VerdictStatusIcon
              status={TestVariantStatus.FLAKY}
              sx={{ verticalAlign: 'middle' }}
            />{' '}
            Flaky Source Verdicts:
          </td>
          <td
            css={{
              color:
                segment.counts.flakyVerdicts > 0
                  ? VERDICT_STATUS_COLOR_MAP[TestVariantStatus.FLAKY]
                  : 'var(--greyed-out-text-color)',
            }}
          >
            {segment.counts.flakyVerdicts}
          </td>
        </tr>
        <tr>
          <td>
            <VerdictStatusIcon
              status={TestVariantStatus.EXPECTED}
              sx={{ verticalAlign: 'middle' }}
            />{' '}
            Expected Source Verdicts:
          </td>
          <td
            css={{
              color:
                expectedVerdictCount > 0
                  ? VERDICT_STATUS_COLOR_MAP[TestVariantStatus.EXPECTED]
                  : 'var(--greyed-out-text-color)',
            }}
          >
            {expectedVerdictCount}
          </td>
        </tr>
      </tbody>
    </table>
  );
}
