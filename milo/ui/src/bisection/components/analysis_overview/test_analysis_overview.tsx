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
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableRow from '@mui/material/TableRow';
import { DateTime } from 'luxon';

import { PlainTable } from '@/bisection/components/plain_table';
import { AnalysisStatusInfo } from '@/bisection/components/status_info';
import {
  ExternalLink,
  linkToBuilder,
  linkToCommit,
} from '@/bisection/tools/link_constructors';
import { GenericNthSectionAnalysisResult } from '@/bisection/types';
import { DurationBadge } from '@/common/components/duration_badge';
import { Timestamp } from '@/common/components/timestamp';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { nthSectionSuspectRange } from './common';

function getSuspectRange(analysis: TestAnalysis): ExternalLink | null {
  if (analysis.culprit?.commit) {
    return linkToCommit(analysis.culprit.commit);
  }
  return analysis.nthSectionResult
    ? nthSectionSuspectRange(
        GenericNthSectionAnalysisResult.fromTest(analysis.nthSectionResult),
      )
    : null;
}

export interface TestAnalysisOverviewProps {
  readonly analysis: TestAnalysis;
}

export function TestAnalysisOverview({ analysis }: TestAnalysisOverviewProps) {
  const builderLink = linkToBuilder(analysis.builder!);
  const suspectRange = getSuspectRange(analysis);
  const failureStartHour = analysis.testFailures[0]
    ? DateTime.fromISO(analysis.testFailures[0].startHour!)
    : null;
  const createTime = analysis.createdTime
    ? DateTime.fromISO(analysis.createdTime)
    : null;
  const endTime = analysis.endTime ? DateTime.fromISO(analysis.endTime) : null;
  return (
    <TableContainer>
      <PlainTable>
        <colgroup>
          <col style={{ width: '20%' }} />
          <col style={{ width: '30%' }} />
          <col style={{ width: '20%' }} />
          <col style={{ width: '30%' }} />
        </colgroup>
        <TableBody data-testid="analysis_overview_table_body">
          <TableRow>
            <TableCell variant="head">Analysis ID</TableCell>
            <TableCell>{analysis.analysisId}</TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant="head">Created time</TableCell>
            <TableCell>
              {createTime && <Timestamp datetime={createTime} />}
            </TableCell>
            <TableCell variant="head">Builder</TableCell>
            <TableCell>
              <Link
                href={builderLink.url}
                target="_blank"
                rel="noreferrer"
                underline="always"
              >
                {builderLink.linkText}
              </Link>
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant="head">End time</TableCell>
            <TableCell>{endTime && <Timestamp datetime={endTime} />}</TableCell>
            <TableCell variant="head">Failure type</TableCell>
            <TableCell>TEST</TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant="head">Status</TableCell>
            <TableCell>
              <AnalysisStatusInfo status={analysis.status}></AnalysisStatusInfo>
            </TableCell>
            <TableCell variant="head">Duration since failure start</TableCell>
            <TableCell>
              {endTime && failureStartHour && (
                <DurationBadge from={failureStartHour} to={endTime} />
              )}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell variant="head">Suspect range</TableCell>
            <TableCell>
              {suspectRange && (
                <span className="span-link">
                  <Link
                    data-testid="analysis_overview_suspect_range"
                    href={suspectRange.url}
                    target="_blank"
                    rel="noreferrer"
                    underline="always"
                  >
                    {suspectRange.linkText}
                  </Link>
                </span>
              )}
            </TableCell>
          </TableRow>
        </TableBody>
      </PlainTable>
    </TableContainer>
  );
}
