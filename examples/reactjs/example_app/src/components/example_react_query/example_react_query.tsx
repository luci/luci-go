// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

import React from 'react';
import { useQuery } from 'react-query';

import Grid from '@mui/material/Grid';
import CircularProgress from '@mui/material/CircularProgress';
import Alert from '@mui/material/Alert';
import { getData } from '../../services/example_service';

const ExampleReactQuery = () => {

    const { data, error, isLoading } = useQuery('example', () => getData());

    if(isLoading) {
        return <CircularProgress />;
    }

    if(error) {
        return <Alert severity='error'> Failed to load data: {error}</Alert>;
    }

     return (
       <Grid container>
         <Grid item>
           {data}
         </Grid>
       </Grid>
     );
};

export default ExampleReactQuery;
