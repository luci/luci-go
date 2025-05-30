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

import { Box, styled } from '@mui/material';
import { ReactNode } from 'react';
import { Helmet } from 'react-helmet';
import { Outlet, UIMatch, useMatches } from 'react-router';
import { useLocalStorage } from 'react-use';

import miloFavicon from '@/common/assets/favicons/milo-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import {
  QueuedStickyScrollingBase,
  StickyOffset,
  Sticky,
} from '@/generic_libs/components/queued_sticky';

import { AppBar } from './app_bar';
import { CookieConsentBar } from './cookie_consent_bar';
import { PrivacyFooter } from './privacy_footer';
import { Sidebar } from './side_bar';
import { VersionBanner } from './version_banner';

const ScrollingBase = styled(QueuedStickyScrollingBase)`
  display: grid;
  min-height: 100vh;
  grid-template-rows: auto auto 1fr auto;
  grid-template-columns: auto 1fr;
  grid-template-areas:
    'banner banner'
    'app-bar app-bar'
    'sidebar main'
    'sidebar footer';
`;

export const SIDE_BAR_OPEN_CACHE_KEY = 'side-bar-open';

export const BaseLayout = () => {
  const [sidebarOpen = true, setSidebarOpen] = useLocalStorage<boolean>(
    SIDE_BAR_OPEN_CACHE_KEY,
  );
  const matches = useMatches() as UIMatch<
    unknown,
    { layout?: () => ReactNode }
  >[];

  // [.at(-1)] If a custom layout is defined by multiple sub routes we want to
  // use the deepest node
  const CustomLayout = matches.filter((m) => m.handle?.layout).at(-1)
    ?.handle?.layout;
  if (CustomLayout) {
    return <CustomLayout />;
  }

  return (
    <ScrollingBase>
      <Helmet titleTemplate="%s | LUCI" defaultTitle="LUCI">
        <link rel="icon" href={miloFavicon} />
      </Helmet>
      <Box sx={{ gridArea: 'banner', zIndex: (theme) => theme.zIndex.appBar }}>
        <VersionBanner />
      </Box>
      <Sticky
        top
        sx={{ gridArea: 'app-bar', zIndex: (theme) => theme.zIndex.appBar }}
      >
        <AppBar open={sidebarOpen} handleSidebarChanged={setSidebarOpen} />
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
        {/* Catch errors here so that when the child do not catch their own
         ** errors, we still render the layout. This is important because the
         ** layout contains potential workarounds to the errors (e.g. login/out
         ** error report, version switches, feature flag toggles, etc).
         **
         ** The child pages should still prefer defining their own error
         ** handlers. See the documentation in `<LoginPage />` for details.
         */}
        <RecoverableErrorBoundary key="base-layout">
          {/* Do not conditionally render the <Outlet /> base on the navigation
           ** state. Otherwise the page state will be reset if a page navigates
           ** to itself, which can happen when the query search param is
           ** updated. In the worst case, this may lead to infinite reload if
           ** the query search param is updated when the component is mounted.
           */}
          <Outlet />
        </RecoverableErrorBoundary>
      </StickyOffset>
      <CookieConsentBar />
    </ScrollingBase>
  );
};
