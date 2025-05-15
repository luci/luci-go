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

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import { useQuery } from '@tanstack/react-query';

import { useAnalysesClient } from '@/common/hooks/prpc_clients';
import { QueryAnalysisRequest } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { AnalysisTableRow } from './table_row';

export interface SearchAnalysisTableProps {
  readonly bbid: string;
}

export function SearchAnalysisTable({ bbid }: SearchAnalysisTableProps) {
  const client = useAnalysesClient();
  const {
    isPending,
    isError,
    isSuccess,
    data: response,
    error,
  } = useQuery({
    ...client.QueryAnalysis.query(
      QueryAnalysisRequest.fromPartial({
        buildFailure: {
          // This query is disabled when bbid is an empty string.
          bbid,
          // TODO: update this once other failure types are analyzed
          failedStepName: 'compile',
        },
      }),
    ),
    // only use the query if a Buildbucket ID has been provided
    enabled: !!bbid,
  });

  if (isPending) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    return (
      <div className="section" data-testid="search-analysis-table">
        <Alert severity="error">
          <AlertTitle>Issue searching by build</AlertTitle>
          {/* TODO: display more error detail for input issues e.g.
                  Build not found, No analysis for that build, etc */}
          An error occurred when searching for analysis using Buildbucket ID
          &quot;
          {bbid}&quot;:
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  let analysis = null;
  let buildIsFirstFailed = false;
  if (isSuccess && response.analyses.length > 0) {
    analysis = response.analyses[0];
    buildIsFirstFailed = analysis.firstFailedBbid === bbid;
  }

  if (!analysis) {
    return (
      <span className="data-placeholder" data-testid="search-analysis-table">
        No analysis found for build {bbid}
      </span>
    );
  }

  return (
    <Box data-testid="search-analysis-table">
      <TableContainer className="analyses-table-container" component={Paper}>
        <Table className="analyses-table" size="small">
          <TableHead>
            <TableRow>
              <TableCell>Buildbucket ID</TableCell>
              <TableCell>Created time</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Failure type</TableCell>
              <TableCell>Duration</TableCell>
              <TableCell>Builder</TableCell>
              <TableCell>Culprit CL</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <AnalysisTableRow analysis={analysis} />
          </TableBody>
        </Table>
      </TableContainer>
      {!buildIsFirstFailed && (
        <div className="section">
          <Alert severity="info">
            <AlertTitle>Found related analysis</AlertTitle>
            The above analysis is related to build {bbid}; there is an earlier
            failed build associated with it.
          </Alert>
        </div>
      )}
    </Box>
  );
}
