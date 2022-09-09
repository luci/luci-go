// Copyright 2022 The LUCI Authors.
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

import { useQuery } from 'react-query';

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';

import { AnalysesTable } from '../components/analyses_table/analyses_table';

import {
  getLUCIBisectionService,
  ListAnalysesRequest,
} from '../services/luci_bisection';

export const FailureAnalysesPage = () => {
  const bisectionService = getLUCIBisectionService();

  // TODO: implement pagination to get older analyses.
  //       Currently, this view will display only the
  //       most recently created failure analyses
  const {
    isLoading,
    isError,
    isSuccess,
    data: response,
    error,
  } = useQuery(['listAnalyses'], async () => {
    const request: ListAnalysesRequest = {};

    return await bisectionService.listAnalyses(request);
  });

  return (
    <main>
      {/* TODO: add a text field to search analyses by Buildbucket ID */}
      {isError && (
        <div className='section'>
          <Alert severity='error'>
            <AlertTitle>Failed to load analyses</AlertTitle>
            {/* TODO: display more error detail for input issues e.g.
                  Build not found, No analysis for that build, etc */}
            An error occurred when querying for existing analyses:
            <Box sx={{ padding: '1rem' }}>{`${error}`}</Box>
          </Alert>
        </div>
      )}
      {isLoading && (
        <Box
          display='flex'
          justifyContent='center'
          alignItems='center'
          height='80vh'
        >
          <CircularProgress />
        </Box>
      )}
      {isSuccess && <AnalysesTable analyses={response.analyses} />}
    </main>
  );
};
