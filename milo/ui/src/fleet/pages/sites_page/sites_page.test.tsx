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

import { given } from './sites_page_given';
import { then } from './sites_page_then';
import { when } from './sites_page_when';

describe('<SitesPage /> (Full Page Integration)', () => {
  it('renders a main landmark container when SitesPage is mounted directly', () => {
    when.pageIsRendered();

    then.mainLandmarkIsVisible();
  });

  it('renders page Component wrapper and tracks analytics when enablePhysicalSites is enabled', async () => {
    given.physicalSitesEnabled();

    when.routeComponentIsRendered();

    await then.mainLandmarkIsRendered();
    then.trackedAnalyticsEvent('page_view', {
      appId: undefined,
      content_group: 'fleet-console-sites',
      page_location: 'http://localhost/',
      page_title: 'Sites | Fleet Console',
    });
  });

  it('renders PageNotFoundPage in Component wrapper when enablePhysicalSites is disabled', async () => {
    given.physicalSitesDisabled();

    when.routeComponentIsRendered();

    await then.textIsVisible('Page not found');
    then.mainLandmarkIsNotVisible();
  });

  it('renders sign-in prompt in Component wrapper when user is unauthenticated', async () => {
    given.physicalSitesEnabled();
    given.unauthenticated();

    when.routeComponentIsRendered();

    await then.textIsVisible('Sign in to Fleet Console');
    then.mainLandmarkIsNotVisible();
  });
});
