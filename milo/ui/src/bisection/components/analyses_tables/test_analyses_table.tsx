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

import './analyses_table.css';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import { TestAnalysis } from '@/common/services/luci_bisection';

import { TestAnalysisTableRow } from './table_row';

interface TestAnalysesTableProps {
  analyses: TestAnalysis[];
}

export function TestAnalysesTable({ analyses }: TestAnalysesTableProps) {
  return (
    <TableContainer className="analyses-table-container" component={Paper}>
      <Table className="analyses-table" size="small">
        <TableHead>
          <TableRow>
            <TableCell>Analysis ID</TableCell>
            <TableCell>Created time</TableCell>
            <TableCell>Duration</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>
              Duration since <br />
              failure start
            </TableCell>
            <TableCell>Builder</TableCell>
            <TableCell>Culprit CL</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {analyses.map((analysis) => (
            <TestAnalysisTableRow
              key={analysis.analysisId}
              analysis={analysis}
            />
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
