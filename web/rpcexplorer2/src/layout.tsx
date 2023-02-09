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

import { Link as RouterLink, Outlet, useParams } from 'react-router-dom';

import AppBar from '@mui/material/AppBar';
import Breadcrumbs from '@mui/material/Breadcrumbs';
import Divider from '@mui/material/Divider';
import Link from '@mui/material/Link';
import Toolbar from '@mui/material/Toolbar';

import NavigateNextIcon from '@mui/icons-material/NavigateNext';

import { GlobalsWaiter } from './context/globals';


const Layout = () => {
  const { serviceName, methodName } = useParams();

  const breadcrumbs: { to: string; title: string }[] = [];
  const addCrumb = (to: string, title: string) => {
    breadcrumbs.push({
      to: to,
      title: title,
    });
  };
  addCrumb('/services/', 'All Services');
  if (serviceName) {
    addCrumb(`/services/${serviceName}`, serviceName);
    if (methodName) {
      addCrumb(`/services/${serviceName}/${methodName}`, methodName);
    }
  }

  return (
    <>
      <AppBar component='nav' position='static'>
        <Toolbar>
          <Link
            variant='h6'
            component={RouterLink}
            to='/services/'
            underline='none'
            color='#ffffff'
            noWrap
          >
            RPC Explorer
          </Link>
          <Divider
            orientation='vertical'
            variant='middle'
            flexItem
            sx={{ pl: 2, borderColor: '#ffffff40' }}
          />
          <Breadcrumbs
            separator={<NavigateNextIcon fontSize='small' />}
            aria-label='breadcrumb'
            sx={{ pl: 2, color: 'white' }}
          >
            {breadcrumbs.map((crumb) => (
              <Link
                underline='hover'
                component={RouterLink}
                color='inherit'
                key={crumb.to}
                to={crumb.to}
              >
                {crumb.title}
              </Link>
            ))}
          </Breadcrumbs>
        </Toolbar>
      </AppBar>
      <GlobalsWaiter>
        <Outlet />
      </GlobalsWaiter>
    </>
  );
};


export default Layout;
