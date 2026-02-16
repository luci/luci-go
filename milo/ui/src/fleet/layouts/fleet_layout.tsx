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
import { Settings } from 'luxon';
import { Outlet, useNavigate } from 'react-router';
import { useLocalStorage } from 'react-use';

import bassFavicon from '@/common/assets/favicons/bass-32.png';
import { SIDE_BAR_OPEN_CACHE_KEY } from '@/common/layouts/base_layout';
import { CookieConsentBar } from '@/common/layouts/cookie_consent_bar';
import { PrivacyFooter } from '@/common/layouts/privacy_footer';
import { VersionBanner } from '@/common/layouts/version_banner';
import { useShortcut } from '@/fleet/components/shortcut_provider';
import {
  CHROMEOS_PLATFORM,
  ANDROID_PLATFORM,
  generateDeviceListURL,
  generateRepairsURL,
  platformToURL,
} from '@/fleet/constants/paths';
import { useCurrentPlatform } from '@/fleet/hooks/usePlatform';
import {
  QueuedStickyScrollingBase,
  Sticky,
  StickyOffset,
} from '@/generic_libs/components/queued_sticky';

import { ShortcutProvider } from '../components/shortcut_provider';
import { IndexedDBPersistClientProvider, SettingsProvider } from '../context';
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

const FleetLayoutContent = () => {
  const [sidebarOpen = false, setSidebarOpen] = useLocalStorage<boolean>(
    SIDE_BAR_OPEN_CACHE_KEY,
  );
  const navigate = useNavigate();
  const currentPlatform = useCurrentPlatform();

  Settings.defaultLocale = 'en';

  const platformStr = currentPlatform ? platformToURL(currentPlatform) : null;

  useShortcut(
    'Go to Devices',
    ['g d', 'g h'],
    () => navigate(generateDeviceListURL(platformStr || CHROMEOS_PLATFORM)),
    { category: 'Navigation' },
  );
  useShortcut(
    'Go to Repairs',
    'g r',
    () => navigate(generateRepairsURL(platformStr || ANDROID_PLATFORM)),
    { category: 'Navigation' },
  );
  useShortcut(
    'Go to Metrics',
    'g m',
    () => navigate('/ui/fleet/labs/metrics'),
    { category: 'Navigation' },
  );
  useShortcut(
    'Go to Requests',
    'g q',
    () => navigate('/ui/fleet/labs/requests'),
    { category: 'Navigation' },
  );
  useShortcut(
    'Go to Planners',
    'g p',
    () => navigate('/ui/fleet/labs/planners'),
    { category: 'Navigation' },
  );
  useShortcut('Collapse Sidebar', '[', () => setSidebarOpen(false), {
    category: 'Global',
  });
  useShortcut('Expand Sidebar', ']', () => setSidebarOpen(true), {
    category: 'Global',
  });

  return (
    <ScrollingBase>
      <link rel="icon" href={bassFavicon} />
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
        <Header sidebarOpen={sidebarOpen} setSidebarOpen={setSidebarOpen} />
      </Sticky>
      <Sticky
        left
        sx={{
          gridRow: '2/5',
          gridColumn: '1/2',
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
  );
};

export const FleetLayout = () => {
  return (
    <IndexedDBPersistClientProvider>
      <ThemeProvider theme={theme}>
        <SettingsProvider>
          <ShortcutProvider>
            <FleetLayoutContent />
          </ShortcutProvider>
        </SettingsProvider>
      </ThemeProvider>
    </IndexedDBPersistClientProvider>
  );
};
