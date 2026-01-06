// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import { Box, Typography } from '@mui/material';
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
import { TestGenAiAnalysisResult } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';

export interface GenAiTestAnalysisTableProps {
  readonly result?: TestGenAiAnalysisResult;
}

export function GenAiTestAnalysisTable({
  result,
}: GenAiTestAnalysisTableProps) {
  if (
    !result ||
    [AnalysisStatus.DISABLED, AnalysisStatus.UNSUPPORTED].includes(
      result.status,
    )
  ) {
    return (
      <span
        className="data-placeholder"
        data-testid="genai-test-analysis-table"
      >
        No AI analysis available for this failure.
      </span>
    );
  }

  if (
    [AnalysisStatus.CREATED, AnalysisStatus.RUNNING].includes(result.status)
  ) {
    return (
      <span
        className="data-placeholder"
        data-testid="genai-test-analysis-table"
      >
        AI analysis is in progress.
      </span>
    );
  }

  if (
    !result.suspects ||
    result.suspects.length === 0 ||
    [AnalysisStatus.NOTFOUND, AnalysisStatus.ERROR].includes(result.status)
  ) {
    return (
      <span
        className="data-placeholder"
        data-testid="genai-test-analysis-table"
      >
        No suspects found by AI Analysis.
      </span>
    );
  }

  return (
    <Box sx={{ p: 2 }}>
      <Typography variant="h6">GenAI Analysis Summary</Typography>
      <Box sx={{ display: 'flex', gap: 2, alignItems: 'center', mb: 2 }}>
        <Typography>
          <b>Status:</b>
        </Typography>
        <AnalysisStatusInfo status={result.status} />
        <Typography>
          <b>Start Time:</b> {getFormattedTimestamp(result.startTime)}
        </Typography>
        <Typography>
          <b>End Time:</b> {getFormattedTimestamp(result.endTime)}
        </Typography>
      </Box>
      <TableContainer
        component={Paper}
        className="genai-table-container"
        data-testid="genai-test-analysis-table"
      >
        <Table className="genai-table" size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ whiteSpace: 'nowrap' }}>Suspect CL</TableCell>
              <TableCell sx={{ whiteSpace: 'nowrap' }}>
                Confidence Score
              </TableCell>
              <TableCell>Justification</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {[...result.suspects]
              .sort((a, b) => b.confidenceScore - a.confidenceScore)
              .map((suspect) => (
                <TableRow key={suspect.reviewUrl}>
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
                    {suspect.confidenceScore.toFixed(0)}
                  </TableCell>
                  <TableCell sx={{ whiteSpace: 'pre-wrap' }}>
                    {suspect.justification}
                  </TableCell>
                </TableRow>
              ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}
