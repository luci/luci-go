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
import TablePagination from '@mui/material/TablePagination';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useQuery, keepPreviousData } from '@tanstack/react-query';
import { useState, useEffect } from 'react';

import { useAnalysesClient } from '@/common/hooks/prpc_clients';
import {
  Analysis,
  ListAnalysesRequest,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';

import { AnalysisTableRow } from './table_row';

export interface DisplayedRowsLabelProps {
  readonly from: number;
}

export function ListAnalysesTable() {
  // TODO: implement sorting & filtering for certain columns

  // The current page of analyses
  const [page, setPage] = useState<number>(0);

  // The page size to use when querying for analyses, and also the number
  // of rows to display in the table
  const [pageSize, setPageSize] = useState<number>(25);

  // A record of page tokens to get the next page of results; the key is the
  // index of the record to continue from
  // TODO: update the key once more query parameters are added
  const [pageTokens, setPageTokens] = useState<Map<number, string>>(
    new Map<number, string>([[0, '']]),
  );

  const client = useAnalysesClient();
  const {
    isPending,
    isError,
    data: response,
    error,
    isFetching,
    isPlaceholderData,
  } = useQuery({
    ...client.ListAnalyses.query(
      ListAnalysesRequest.fromPartial({
        pageSize: pageSize,
        pageToken: pageTokens.get(page * pageSize) || '',
      }),
    ),
    placeholderData: keepPreviousData,
  });

  useEffect(() => {
    // Record the page token for the next page of analyses
    const nextPageStartIndex = (page + 1) * pageSize;
    if (
      response &&
      response.nextPageToken !== null &&
      !pageTokens.has(nextPageStartIndex)
    ) {
      setPageTokens(
        new Map(pageTokens.set(nextPageStartIndex, response.nextPageToken)),
      );
    }
  }, [page, pageSize, pageTokens, response]);

  const analyses: readonly Analysis[] = response?.analyses || [];

  const handleChangePage = (_: React.MouseEvent | null, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    // Set the new page size then reset to the first page of results
    setPageSize(parseInt(event.target.value, 10));
    setPage(0);
  };

  const labelDisplayedRows = ({ from }: DisplayedRowsLabelProps) => {
    if (analyses) {
      return `${from}-${from + analyses.length - 1}`;
    }
    return '';
  };

  if (isPending) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center">
        <CircularProgress />
      </Box>
    );
  }

  if (isError) {
    return (
      <div className="section" data-testid="list-analyses-table">
        <Alert severity="error">
          <AlertTitle>Failed to load analyses</AlertTitle>
          {/* TODO: display more error detail for input issues e.g.
                    Build not found, No analysis for that build, etc */}
          An error occurred when querying for existing analyses:
          <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
        </Alert>
      </div>
    );
  }

  if (analyses.length === 0) {
    return (
      <span className="data-placeholder" data-testid="list-analyses-table">
        No analyses
      </span>
    );
  }

  const nextPageToken = pageTokens.get((page + 1) * pageSize);
  const isLastPage = nextPageToken === undefined || nextPageToken === '';
  return (
    <Box data-testid="list-analyses-table">
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
            {analyses.map((analysis) => (
              <AnalysisTableRow key={analysis.analysisId} analysis={analysis} />
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <>
        {!isFetching || !isPlaceholderData ? (
          <TablePagination
            component="div"
            count={-1}
            page={page}
            onPageChange={handleChangePage}
            rowsPerPage={pageSize}
            rowsPerPageOptions={[25, 50, 100, 200]}
            onRowsPerPageChange={handleChangeRowsPerPage}
            labelDisplayedRows={labelDisplayedRows}
            // disable the "next" button if there are no more analyses
            slotProps={{
              actions: {
                nextButton: {
                  disabled: isLastPage,
                },
              },
            }}
          />
        ) : (
          <Box
            sx={{ padding: '1rem' }}
            display="flex"
            justifyContent="right"
            alignItems="center"
          >
            <CircularProgress size="1.25rem" />
            <Box sx={{ paddingLeft: '1rem' }}>
              <Typography variant="caption">Fetching analyses...</Typography>
            </Box>
          </Box>
        )}
      </>
    </Box>
  );
}
