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

import CheckCircleOutline from '@mui/icons-material/CheckCircleOutline';
import IconButton from '@mui/material/IconButton';
import Link from '@mui/material/Link';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

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

  if (
    [AnalysisStatus.CREATED, AnalysisStatus.RUNNING].includes(result.status)
  ) {
    return (
      <span className="data-placeholder" data-testid="genai-analysis-table">
        AI analysis is in progress.
      </span>
    );
  }

  if (
    !result.suspect ||
    [AnalysisStatus.NOTFOUND, AnalysisStatus.ERROR].includes(result.status)
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
            <TableCell>Suspect CL</TableCell>
            <TableCell>Verified</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          <TableRow>
            <TableCell>
              <Link
                href={result.suspect.reviewUrl}
                target="_blank"
                rel="noreferrer"
                underline="always"
              >
                {result.suspect.reviewTitle}
              </Link>
            </TableCell>
            <TableCell>
              {result.suspect.verified && (
                <IconButton style={{ color: 'var(--success-color)' }}>
                  <CheckCircleOutline />
                </IconButton>
              )}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </TableContainer>
  );
}
