// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.


import './example.css';

import Grid from '@mui/material/Grid';

import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import {
  useGetExampleByNameQuery,
} from '../../services/example_service_rtkquery';
import { useAppSelector } from '../../store/custom_hooks';

interface Props {
    exampleProp: string;
}

const Example = ({
  exampleProp,
}: Props) => {
  const count = useAppSelector((state) => state.counter.value);

  const { data, error, isLoading } = useGetExampleByNameQuery('user');

  if (isLoading) {
    return <CircularProgress />;
  }

  if (error) {
    return <Alert severity='error'> Failed to load data: {error}</Alert>;
  }

  return (
    <div className="example" role="article">
      {exampleProp}
      <Grid>
        {count}
      </Grid>
      <Grid>
        {data}
      </Grid>
    </div>
  );
};

export default Example;
