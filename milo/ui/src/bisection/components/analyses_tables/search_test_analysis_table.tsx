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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import { useQuery } from '@tanstack/react-query';

import { useAnalysesClient } from '@/bisection/hooks/prpc_clients';
import { GetTestAnalysisRequest } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { TestAnalysesTable } from './test_analyses_table';

export interface SearchTestAnalysisTableProps {
  readonly analysisId: string;
}

export function SearchTestAnalysisTable({
  analysisId,
}: SearchTestAnalysisTableProps) {
  const client = useAnalysesClient();
  const {
    isLoading,
    isError,
    data: analysis,
    error,
  } = useQuery({
    ...client.GetTestAnalysis.query(
      GetTestAnalysisRequest.fromPartial({
        analysisId,
      }),
    ),
    // only use the query if a Analysis ID has been provided
    enabled: !!analysisId,
  });

  if (isLoading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    if (error instanceof GrpcError && error.code === RpcCode.NOT_FOUND) {
      return (
        <span className="data-placeholder" data-testid="search-analysis-table">
          No analysis found for ID {analysisId}
        </span>
      );
    }
    return (
      <div className="section" data-testid="search-analysis-table">
        <Alert severity="error">
          <AlertTitle>Issue searching by ID</AlertTitle>
          {/* TODO: display more error detail for input issues e.g.
                  Build not found, No analysis for that ID, etc */}
          An error occurred when searching for analysis using Analysis ID &quot;
          {analysisId}&quot;:
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  return (
    <Box data-testid="search-analysis-table">
      <TestAnalysesTable analyses={[analysis]} />
    </Box>
  );
}
