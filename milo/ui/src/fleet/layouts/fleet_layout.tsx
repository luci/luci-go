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

import { Outlet } from 'react-router-dom';
import { useLocalStorage } from 'react-use';

import { SIDE_BAR_OPEN_CACHE_KEY } from '@/common/layouts/base_layout';
import { CookieConsentBar } from '@/common/layouts/cookie_consent_bar';
import { PrivacyFooter } from '@/common/layouts/privacy_footer';

import { Header } from './header';
import { Sidebar } from './sidebar';

export const FleetLayout = () => {
  const [sidebarOpen = false, setSidebarOpen] = useLocalStorage<boolean>(
    SIDE_BAR_OPEN_CACHE_KEY,
  );

  return (
    <div
      css={{
        minHeight: '100vh',
        minWidth: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <Header sidebarOpen={sidebarOpen} setSidebarOpen={setSidebarOpen} />
      <Sidebar open={sidebarOpen} />
      <Outlet />

      <div
        css={{
          marginTop: 'auto',
        }}
      >
        <PrivacyFooter />
      </div>
      <CookieConsentBar />
    </div>
  );
};
