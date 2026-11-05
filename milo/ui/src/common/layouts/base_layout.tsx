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
import { Outlet, UIMatch, useMatches } from 'react-router';
import { useLocalStorage } from 'react-use';

import miloFavicon from '@/common/assets/favicons/milo-32.png';
import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useProjectCtx } from '@/common/components/page_meta';
import {
  QueuedStickyScrollingBase,
  StickyOffset,
  Sticky,
} from '@/generic_libs/components/queued_sticky';

import { AppBar } from './app_bar';
import { DEFAULT_APP_NAME } from './app_bar/app_details';
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

/**
 * Layout options that sub-apps can declare in their route `handle`.
 * See `docs/guides/customizing_layouts.md` for details.
 */
export interface LayoutHandle {
  /**
   * The link target of the app name in the app bar. Defaults to `/ui/`.
   */
  readonly appHomeLink?: string;
  /**
   * The app name displayed in the app bar. Also used as the default browser
   * tab title. Defaults to `LUCI`.
   */
  readonly appName?: string;
  /**
   * The initial open state of the sidebar when the user has no stored
   * preference.
   */
  readonly defaultSidebarOpen?: boolean;
  /**
   * Hides the "more" (⋮) app menu in the app bar.
   */
  readonly hideAppMenu?: boolean;
  /**
   * Hides the feature flags button in the app bar.
   */
  readonly hideFeatureFlags?: boolean;
  /**
   * Hides the global "Send feedback" button in the app bar.
   */
  readonly hideGlobalFeedback?: boolean;
  /**
   * Hides the LUCI project breadcrumb in the app bar. Useful for sub-apps that
   * are not scoped to a LUCI project, since the project context is retained
   * across SPA navigations.
   */
  readonly hideProject?: boolean;
  /**
   * Hides the sidebar and the sidebar toggle in the app bar.
   */
  readonly hideSidebar?: boolean;
  /**
   * A component that completely replaces the `BaseLayout`.
   */
  readonly layout?: () => ReactNode;
}

export const BaseLayout = () => {
  const [sidebarOpen, setSidebarOpen] = useLocalStorage<boolean>(
    SIDE_BAR_OPEN_CACHE_KEY,
  );
  const project = useProjectCtx();
  // Sub-apps can customize their parent layouts by exporting properties in their
  // route handles. For more details, see `docs/guides/customizing_layouts.md`.
  const matches = useMatches() as UIMatch<unknown, LayoutHandle>[];

  // [.at(-1)] If a property is defined by multiple sub routes we want to use
  // the value from the deepest node.
  const getHandleValue = <K extends keyof LayoutHandle>(
    key: K,
  ): LayoutHandle[K] | undefined =>
    matches.filter((m) => m.handle?.[key] !== undefined).at(-1)?.handle?.[key];

  const defaultSidebarOpen = getHandleValue('defaultSidebarOpen');
  const hideGlobalFeedback = getHandleValue('hideGlobalFeedback');
  const hideAppMenu = getHandleValue('hideAppMenu');
  const hideFeatureFlags = getHandleValue('hideFeatureFlags');
  const hideProject = getHandleValue('hideProject');
  const hideSidebar = getHandleValue('hideSidebar') ?? false;
  const appName = getHandleValue('appName') ?? DEFAULT_APP_NAME;
  const appHomeLink = getHandleValue('appHomeLink');

  const shouldShowSidebar = project !== 'android' && !hideSidebar;
  const effectiveSidebarOpen = sidebarOpen ?? defaultSidebarOpen ?? false;

  const CustomLayout = getHandleValue('layout');
  if (CustomLayout) {
    return <CustomLayout />;
  }

  return (
    <ScrollingBase>
      <link rel="icon" href={miloFavicon} />
      {/* React hoists <title> into <head> and inserts each newly mounted
       ** <title> before any existing ones, so the most recently mounted title
       ** is the one shown in the browser tab. Pages under this layout mount
       ** after it, so they can still render their own <title> (e.g.
       ** "Dashboard X | CrystalBall") to override this app-level default.
       **
       ** Always render this <title> (defaulting to `DEFAULT_APP_NAME`) so it is
       ** mounted once and its text is updated in place across route
       ** transitions, rather than being mounted/unmounted as `appName` changes.
       */}
      <title>{appName}</title>
      <Box sx={{ gridArea: 'banner', zIndex: (theme) => theme.zIndex.appBar }}>
        <VersionBanner />
      </Box>
      <Sticky
        top
        sx={{ gridArea: 'app-bar', zIndex: (theme) => theme.zIndex.appBar }}
      >
        <AppBar
          open={effectiveSidebarOpen}
          handleSidebarChanged={setSidebarOpen}
          showSidebarToggle={shouldShowSidebar}
          hideGlobalFeedback={hideGlobalFeedback}
          hideAppMenu={hideAppMenu}
          hideFeatureFlags={hideFeatureFlags}
          hideProject={hideProject}
          appName={appName}
          appHomeLink={appHomeLink}
        />
      </Sticky>
      {!hideSidebar && (
        <Sticky
          left
          sx={{
            gridRow: '2/5',
            gridColumn: '1/2',
            zIndex: (theme) => theme.zIndex.drawer,
          }}
        >
          <Sidebar open={shouldShowSidebar && effectiveSidebarOpen} />
        </Sticky>
      )}
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
