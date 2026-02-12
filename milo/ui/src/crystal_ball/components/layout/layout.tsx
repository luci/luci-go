// Copyright 2026 The LUCI Authors.
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

import Box from '@mui/material/Box';
import { Outlet } from 'react-router';

import { Sticky } from '@/generic_libs/components/queued_sticky';

import { TopBar } from './top_bar';
import { TopBarProvider } from './top_bar_provider';

/**
 * The main layout component for CrystalBall pages.
 * Includes the TopBar and a fluid main content area.
 */
export function Layout() {
  return (
    <TopBarProvider>
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          minHeight: '100vh',
          backgroundColor: 'grey.50',
        }}
      >
        <Sticky top sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
          <TopBar />
        </Sticky>
        <Box component="main" sx={{ flexGrow: 1 }}>
          <Outlet />
        </Box>
      </Box>
    </TopBarProvider>
  );
}

export const Component = Layout;
