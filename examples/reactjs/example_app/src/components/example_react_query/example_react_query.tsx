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

import Grid from '@mui/material/Grid';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import { getData } from '@/services/example_service';

const ExampleReactQuery = () => {
  const { data, error, isLoading } = useQuery('example', () => getData());

  if (isLoading) {
    return <CircularProgress />;
  }

  if (error) {
    return (
      <Alert severity='error'>
        <>Failed to load data: {error}</>
      </Alert>
    );
  }

  return (
    <Grid container>
      <Grid item>
        <>
          {data}
        </>
      </Grid>
    </Grid>
  );
};

export default ExampleReactQuery;
