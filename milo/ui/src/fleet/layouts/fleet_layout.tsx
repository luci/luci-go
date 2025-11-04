// Copyright 2024 The LUCI Authors.
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

import { Box, ThemeProvider, styled } from '@mui/material';
import { NotificationsProvider } from '@toolpad/core/useNotifications';
import { Settings } from 'luxon';
import { Helmet } from 'react-helmet';
import { Outlet } from 'react-router';
import { useLocalStorage } from 'react-use';

import bassFavicon from '@/common/assets/favicons/bass-32.png';
import { SIDE_BAR_OPEN_CACHE_KEY } from '@/common/layouts/base_layout';
import { CookieConsentBar } from '@/common/layouts/cookie_consent_bar';
import { PrivacyFooter } from '@/common/layouts/privacy_footer';
import { VersionBanner } from '@/common/layouts/version_banner';
import {
  QueuedStickyScrollingBase,
  StickyOffset,
  Sticky,
} from '@/generic_libs/components/queued_sticky';

import { IndexedDBPersistClientProvider } from '../context';
import { theme } from '../theme/theme';

import { Header } from './header';
import { Sidebar } from './sidebar';

const ScrollingBase = styled(QueuedStickyScrollingBase)`
  display: grid;
  min-height: 100vh;
  grid-template-rows: auto auto 1fr auto;
  grid-template-columns: auto minmax(0, 1fr);
  grid-template-areas:
    'banner banner'
    'header header'
    'sidebar main'
    'sidebar footer';
`;

export const FleetLayout = () => {
  const [sidebarOpen = false, setSidebarOpen] = useLocalStorage<boolean>(
    SIDE_BAR_OPEN_CACHE_KEY,
  );

  Settings.defaultLocale = 'en';

  return (
    <IndexedDBPersistClientProvider>
      <NotificationsProvider>
        <ThemeProvider theme={theme}>
          <ScrollingBase>
            <Helmet
              titleTemplate="%s | Fleet Console"
              defaultTitle="Fleet Console"
            >
              <link rel="icon" href={bassFavicon} />
            </Helmet>
            <Box
              sx={{
                gridArea: 'banner',
                zIndex: (theme) => theme.zIndex.appBar,
              }}
            >
              <VersionBanner />
            </Box>
            <Sticky
              top
              sx={{
                gridArea: 'header',
                zIndex: (theme) => theme.zIndex.appBar,
              }}
            >
              <Header
                sidebarOpen={sidebarOpen}
                setSidebarOpen={setSidebarOpen}
              />
            </Sticky>
            <Sticky
              left
              sx={{
                gridArea: 'sidebar',
                zIndex: (theme) => theme.zIndex.drawer,
              }}
            >
              <Sidebar open={sidebarOpen} />
            </Sticky>
            <Sticky top sx={{ gridArea: 'footer' }}>
              <PrivacyFooter />
            </Sticky>
            <StickyOffset component="main" sx={{ gridArea: 'main' }}>
              <Outlet />
            </StickyOffset>
            <CookieConsentBar />
          </ScrollingBase>
        </ThemeProvider>
      </NotificationsProvider>
    </IndexedDBPersistClientProvider>
  );
};
