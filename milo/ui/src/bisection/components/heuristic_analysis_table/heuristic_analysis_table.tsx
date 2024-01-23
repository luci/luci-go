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

import './heuristic_analysis_table.css';

import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';

import {
  HeuristicAnalysisResult,
  isAnalysisComplete,
} from '@/common/services/luci_bisection';

import { HeuristicAnalysisTableRow } from './heuristic_analysis_table_row';

interface Props {
  result?: HeuristicAnalysisResult;
}

export function HeuristicAnalysisTable({ result }: Props) {
  if (!result) {
    return (
      <span className="data-placeholder" data-testid="heuristic-analysis-table">
        There is no heuristic analysis
      </span>
    );
  }

  if (!isAnalysisComplete(result.status)) {
    return (
      <span className="data-placeholder" data-testid="heuristic-analysis-table">
        Heuristic analysis is in progress
      </span>
    );
  }

  if (!result.suspects || result.suspects.length === 0) {
    return (
      <span className="data-placeholder" data-testid="heuristic-analysis-table">
        No suspects found
      </span>
    );
  }

  return (
    <TableContainer
      component={Paper}
      className="heuristic-table-container"
      data-testid="heuristic-analysis-table"
    >
      <Table className="heuristic-table" size="small">
        <TableHead>
          <TableRow>
            <TableCell>Suspect CL</TableCell>
            <TableCell>Confidence</TableCell>
            <TableCell>Score</TableCell>
            <TableCell>Justification</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {result.suspects.map((suspect) => (
            <HeuristicAnalysisTableRow
              key={suspect.gitilesCommit.id}
              suspect={suspect}
            />
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
}
