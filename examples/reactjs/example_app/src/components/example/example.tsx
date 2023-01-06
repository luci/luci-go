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


import './example.css';

import Grid from '@mui/material/Grid';

import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import {
  useGetExampleByNameQuery,
} from '@/services/example_service_rtkquery';
import { useAppSelector } from '@/store/custom_hooks';

interface Props {
    exampleProp: string;
}

const Example = ({
  exampleProp,
}: Props) => {
  const count = useAppSelector((state) => state.counter.value);

  const { data, error, isLoading } = useGetExampleByNameQuery('mama');
  if (isLoading) {
    return <CircularProgress />;
  }

  if (error) {
    return (
      <Alert severity='error'>
        <>Failed to load data</>
      </Alert>
    );
  }

  return (
    <div className="example" role="article">
      {exampleProp}
      <Grid>
        {count}
      </Grid>
      <Grid>
        <>
          {data && 'loaded'}
        </>
      </Grid>
    </div>
  );
};

export default Example;
