// Copyright 2023 The LUCI Authors.
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

import Link from '@mui/material/Link';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import { DateTime } from 'luxon';
import { Link as RouterLink } from 'react-router-dom';

import { ANALYSIS_STATUS_DISPLAY_MAP } from '@/bisection/constants';
import { linkToBuilder } from '@/bisection/tools/link_constructors';
import { GenericCulprit } from '@/bisection/types';
import { DurationBadge } from '@/common/components/duration_badge';
import { Timestamp } from '@/common/components/timestamp';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { CulpritsTableCell } from './culprit_table_cell';

export interface TestAnalysisTableRowProps {
  readonly analysis: TestAnalysis;
}

export function TestAnalysisTableRow({ analysis }: TestAnalysisTableRowProps) {
  const builderLink = analysis.builder ? linkToBuilder(analysis.builder) : null;

  const failureStartHour = analysis.testFailures[0]?.startHour
    ? DateTime.fromISO(analysis.testFailures[0].startHour)
    : null;
  const createTime = analysis.createdTime
    ? DateTime.fromISO(analysis.createdTime)
    : null;
  const endTime = analysis.endTime ? DateTime.fromISO(analysis.endTime) : null;
  const totalDuration =
    endTime && failureStartHour ? endTime.diff(failureStartHour) : null;
  const runDuration = createTime && endTime ? endTime.diff(createTime) : null;

  return (
    <TableRow hover data-testid="analysis_table_row">
      <TableCell>
        <Link
          component={RouterLink}
          to={`/ui/bisection/test-analysis/b/${analysis.analysisId}`}
          data-testid="analysis_table_row_analysis_link"
        >
          {analysis.analysisId}
        </Link>
      </TableCell>
      <TableCell>{createTime && <Timestamp datetime={createTime} />}</TableCell>
      <TableCell>
        {runDuration && (
          <DurationBadge
            duration={runDuration}
            from={createTime}
            to={endTime}
          />
        )}
      </TableCell>
      <TableCell>{ANALYSIS_STATUS_DISPLAY_MAP[analysis.status]}</TableCell>
      <TableCell>
        {totalDuration && (
          <DurationBadge
            duration={totalDuration}
            from={failureStartHour}
            to={endTime}
          />
        )}
      </TableCell>
      <TableCell>
        {builderLink && (
          <Link
            href={builderLink.url}
            target="_blank"
            rel="noreferrer"
            underline="always"
            data-testid="analysis_table_row_builder_link"
          >
            {builderLink.linkText}
          </Link>
        )}
      </TableCell>
      <CulpritsTableCell
        culprits={
          analysis.culprit ? [GenericCulprit.fromTest(analysis.culprit)] : []
        }
        status={analysis.status}
      />
    </TableRow>
  );
}
