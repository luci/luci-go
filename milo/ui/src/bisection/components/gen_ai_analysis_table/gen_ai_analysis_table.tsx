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

import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { AnalysisStatusInfo } from '@/bisection/components/status_info';
import { getFormattedTimestamp } from '@/bisection/tools/timestamp_formatters';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { GenAiAnalysisResult } from '@/proto/go.chromium.org/luci/bisection/proto/v1/genai.pb';
export interface GenAiAnalysisTableProps {
  readonly result?: GenAiAnalysisResult;
}

export function GenAiAnalysisTable({ result }: GenAiAnalysisTableProps) {
  if (
    !result ||
    [AnalysisStatus.DISABLED, AnalysisStatus.UNSUPPORTED].includes(
      result.status,
    )
  ) {
    return (
      <span className="data-placeholder" data-testid="genai-analysis-table">
        No AI analysis available for this failure.
      </span>
    );
  }
  const { status, suspect, startTime, endTime } = result;
  if ([AnalysisStatus.CREATED, AnalysisStatus.RUNNING].includes(status)) {
    return (
      <span className="data-placeholder" data-testid="genai-analysis-table">
        AI analysis is in progress.
      </span>
    );
  }

  if (
    !suspect ||
    [AnalysisStatus.NOTFOUND, AnalysisStatus.ERROR].includes(status)
  ) {
    return (
      <span className="data-placeholder" data-testid="genai-analysis-table">
        No suspects found by AI Analysis.
      </span>
    );
  }
  return (
    <TableContainer
      component={Paper}
      className="genai-table-container"
      data-testid="genai-analysis-table"
    >
      <Table className="genai-table" size="small">
        <TableHead>
          <TableRow>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>Suspect CL</TableCell>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>Status</TableCell>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>Start Time</TableCell>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>End Time</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          <TableRow>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>
              <Link
                href={suspect.reviewUrl}
                target="_blank"
                rel="noreferrer"
                underline="always"
              >
                {suspect.reviewTitle}
              </Link>
            </TableCell>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>
              <AnalysisStatusInfo status={result.status}></AnalysisStatusInfo>
            </TableCell>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>
              {getFormattedTimestamp(startTime)}
            </TableCell>
            <TableCell sx={{ whiteSpace: 'nowrap' }}>
              {getFormattedTimestamp(endTime)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell colSpan={4} sx={{ whiteSpace: 'pre-wrap' }}>
              <b>Justification:</b>
              <div style={{ paddingTop: '5px' }}>{suspect.justification}</div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </TableContainer>
  );
}
