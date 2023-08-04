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

import { styled } from '@mui/material';
import Box from '@mui/material/Box';
import { Outlet } from 'react-router-dom';

import { ErrorBoundary } from '@/common/components/error_boundary';
import { useLocalStorageItem } from '@/common/hooks/use_local_storage_item';

import { AppBar } from './app_bar';
import { drawerWidth } from './constants';
import { Sidebar } from './side_bar';

const Main = styled('main', { shouldForwardProp: (prop) => prop !== 'open' })<{
  open?: boolean;
}>(({ theme, open }) => ({
  flexGrow: 1,
  transition: theme.transitions.create('margin', {
    easing: theme.transitions.easing.sharp,
    duration: theme.transitions.duration.leavingScreen,
  }),
  marginLeft: `-${drawerWidth}px`,
  ...(open && {
    transition: theme.transitions.create('margin', {
      easing: theme.transitions.easing.easeOut,
      duration: theme.transitions.duration.enteringScreen,
    }),
    marginLeft: 0,
  }),
}));

export const SIDE_BAR_OPEN_CACHE_KEY = 'side-bar-open';

export const BaseLayout = () => {
  const [sidebarOpenCache, setSidebarOpenCache] = useLocalStorageItem(
    SIDE_BAR_OPEN_CACHE_KEY,
    'true'
  );
  const sidebarOpen = sidebarOpenCache === 'true';

  const handleSidebarChanged = (isOpen: boolean) => {
    setSidebarOpenCache(String(isOpen));
  };

  return (
    <ErrorBoundary>
      <Box sx={{ display: 'flex', pt: 7 }}>
        <AppBar
          open={sidebarOpen}
          handleSidebarChanged={handleSidebarChanged}
        />
        <Sidebar open={sidebarOpen} />
        <Main open={sidebarOpen}>
          {/*
           ** <AppBar /> and the <SideBar /> supports useful actions in case of
           ** an error (e.g. file a bug, log in/out).
           ** Wraps <Outlet /> in a separate <ErrorBoundary /> to ensure the
           ** <AppBar /> and the <SideBar> are always displayed when possible.
           */}
          <ErrorBoundary>
            <Outlet />
          </ErrorBoundary>
        </Main>
      </Box>
    </ErrorBoundary>
  );
};
