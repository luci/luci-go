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

import { GrpcError } from '@chopsui/prpc-client';
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

import {
  useAnalysesClient,
  useBuildsClient,
} from '@/common/hooks/prpc_clients';
import { QueryAnalysisRequest } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { GetBuildRequest } from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { Status as BuildStatus } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { AnalysisTableRow } from './table_row';

export interface SearchAnalysisTableProps {
  readonly bbid: string;
}

export function SearchAnalysisTable({ bbid }: SearchAnalysisTableProps) {
  const buildsClient = useBuildsClient();
  const {
    data: build,
    isPending: isBuildPending,
    isError: isBuildError,
    error: buildError,
  } = useQuery({
    ...buildsClient.GetBuild.query(
      GetBuildRequest.fromPartial({
        id: bbid,
        mask: {
          fields: ['id', 'steps'],
        },
      }),
    ),
    staleTime: 5000,
    enabled: true,
  });

  const failedStepName = build?.steps.find(
    (s) => s.status === BuildStatus.FAILURE,
  )?.name;

  const isAnalysisQueryEnabled = !!(bbid && failedStepName);

  const analysesClient = useAnalysesClient();
  const {
    isPending: isAnalysisPending,
    isError,
    isSuccess,
    data: response,
    error,
  } = useQuery({
    ...analysesClient.QueryAnalysis.query(
      QueryAnalysisRequest.fromPartial({
        buildFailure: {
          bbid,
          failedStepName: failedStepName,
        },
      }),
    ),
    // only use the query if a Buildbucket ID has been provided and a failed
    // step has been found
    enabled: isAnalysisQueryEnabled,
  });

  if (isBuildPending || (isAnalysisQueryEnabled && isAnalysisPending)) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isBuildError) {
    if (buildError instanceof GrpcError && buildError.code === 5) {
      // 5 = NOT_FOUND
      return (
        <div className="section" data-testid="search-analysis-table">
          <Alert severity="warning">Build {bbid} not found</Alert>
        </div>
      );
    }
    return (
      <div className="section" data-testid="search-analysis-table">
        <Alert severity="error">
          <AlertTitle>Error fetching build {bbid}</AlertTitle>
          <Box sx={{ padding: '1rem' }}>{`${buildError}`}</Box>
        </Alert>
      </div>
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
