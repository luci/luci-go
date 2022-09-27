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


import './base.css';

import { Link, matchPath, Outlet, useLocation } from 'react-router-dom';

import AppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Grid from '@mui/material/Grid';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Toolbar from '@mui/material/Toolbar';

declare global {
  interface Window {
    avatar: string;
    email: string;
    fullName: string;
    logoutURL: string;
  }
}

function getCurrentLink(linkPatterns: string[]) {
  const { pathname } = useLocation();

  for (let i = 0; i < linkPatterns.length; i += 1) {
    const linkMatch = matchPath(linkPatterns[i], pathname);
    if (linkMatch !== null) {
      return linkMatch;
    }
  }

  return null;
}

export const BaseLayout = () => {
  const linkMatcher = getCurrentLink(['/trigger', '/statistics', '/']);

  var currentTab = '/';
  if (linkMatcher !== null) {
    currentTab = linkMatcher?.pattern?.path;
  }

  return (
    <Box sx={{ flexGrow: 1 }}>
      <AppBar position='static' color='primary'>
        <Toolbar>
          <Tabs value={currentTab} textColor='inherit'>
            <Tab
              className='logo-nav-tab'
              component={Link}
              label='LUCI Bisection'
              value='/'
              to='/'
            />
            <Tab
              className='nav-tab'
              component={Link}
              label='New Analysis'
              value='/trigger'
              to='/trigger'
              color='inherit'
              // TODO: remove below once the New Analysis page is implemented
              disabled
            />
            <Tab
              className='nav-tab'
              component={Link}
              label='Statistics'
              value='/statistics'
              to='/statistics'
              color='inherit'
              // TODO: remove below once the Statistics page is implemented
              disabled
            />
          </Tabs>
          {/* TODO: add login/logout links */}
        </Toolbar>
      </AppBar>
      <Container maxWidth={false} sx={{ marginTop: '2rem' }}>
        <Grid container justifyContent='center'>
          <Grid item xs={12} xl={9}>
            <Outlet />
          </Grid>
        </Grid>
      </Container>
    </Box>
  );
};
