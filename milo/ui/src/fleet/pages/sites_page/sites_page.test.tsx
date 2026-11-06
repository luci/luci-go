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
  it('renders an empty state when no physical sites exist', async () => {
    given.sites([]);

    when.pageIsRendered();

    then.mainLandmarkIsVisible();
    await then.textIsVisible('No physical sites registered yet.');
  });

  it('renders physical site cards ordered deterministically by name and id', async () => {
    given.sites([
      { id: 'MTV', name: 'Mountain View MTV' },
      { id: 'LAB2', name: 'Central Lab' },
      { id: 'LAB1', name: 'Central Lab' },
      { id: 'EM25', name: 'Palo Alto EM25' },
    ]);

    when.pageIsRendered();

    await then.siteCardIsVisible('Central Lab', 'LAB1');
    await then.siteCardIsVisible('Central Lab', 'LAB2');
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');
    await then.siteCardHasLink(
      'EM25',
      'Open EM25 in Campus Maps',
      'https://goto.google.com/where/?id=EM25',
    );
    await then.siteCardIdOrderIs(['LAB1', 'LAB2', 'MTV', 'EM25']);
    await then.textIsVisible('4 sites');
  });

  it('filters physical site cards by name or building code and supports clearing the filter', async () => {
    given.sites([
      { id: 'MTV', name: 'Mountain View MTV' },
      { id: 'EM25', name: 'Palo Alto EM25' },
    ]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');

    await when.userSearchesSites('EM25');

    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');
    then.siteCardIsNotVisible('Mountain View MTV');
    await then.textIsVisible('1 site');

    await when.userSearchesSites('nonexistent');

    await then.textIsVisible('No sites match "nonexistent".');
    await when.userClicksButton('Clear filter');

    await then.siteCardIsVisible('Mountain View MTV', 'MTV');
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');
  });

  it('creates a new physical site and refreshes the sorted card grid', async () => {
    given.sites([{ id: 'MTV', name: 'Mountain View MTV' }]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');

    await when.userOpensAddSiteDialog();
    await when.userFillsSiteForm({ id: 'EM25', name: 'Palo Alto EM25' });
    await when.userSubmitsSiteForm();

    then.rpcWasCalledWith('CreatePhysicalSite', {
      physicalSite: { id: 'EM25', name: 'Palo Alto EM25' },
    });
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');
    await then.siteCardOrderIs(['Mountain View MTV', 'Palo Alto EM25']);
  });

  it('updates an existing physical site name while keeping site code immutable', async () => {
    given.sites([{ id: 'EM25', name: 'Palo Alto' }]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Palo Alto', 'EM25');

    await when.userOpensEditSiteDialog('Palo Alto');
    await then.siteIdInputIsDisabled();
    await when.userFillsSiteForm({ name: 'Palo Alto EM25' });
    await when.userSubmitsSiteForm();

    then.rpcWasCalledWith('UpdatePhysicalSite', {
      physicalSite: { id: 'EM25', name: 'Palo Alto EM25' },
    });
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');
  });

  it('deletes a physical site after confirmation', async () => {
    given.sites([
      { id: 'MTV', name: 'Mountain View MTV' },
      { id: 'EM25', name: 'Palo Alto EM25' },
    ]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');

    await when.userOpensDeleteSiteDialog('Palo Alto EM25');
    await when.userConfirmsDeleteSite();

    then.rpcWasCalledWith('DeletePhysicalSite', { id: 'EM25' });
    await then.siteCardIsRemoved('Palo Alto EM25');
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');
  });

  it('prevents duplicate site codes and surfaces server errors in the form dialog', async () => {
    given.sites([{ id: 'MTV', name: 'Mountain View MTV' }]);
    given.createSiteError('Site write rejected');

    when.pageIsRendered();
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');

    await when.userOpensAddSiteDialog();
    await when.userFillsSiteForm({ id: 'MTV', name: 'Duplicate Code' });
    await then.textIsVisible('A site with this code already exists.');
    await then.siteFormSubmitIsDisabled();

    await when.userFillsSiteForm({ id: 'EM25', name: 'Palo Alto EM25' });
    await when.userSubmitsSiteForm();

    await then.textIsVisible(/Site write rejected/);
  });

  it('prompts for admin access when a user without admin task permission clicks Add or Delete site', async () => {
    given.sites([{ id: 'MTV', name: 'Mountain View MTV' }]);
    given.withoutAdminTaskPermission();

    when.pageIsRendered();
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');

    await when.userClicksButton('Add site');
    await then.textIsVisible('Admin access required');
    await then.textIsVisible('fleet-console-admin-tasks-policy');
    await when.userClicksButton('Close');

    await when.userOpensEditSiteDialog('Mountain View MTV');
    await then.textIsVisible('Admin access required');
    await when.userClicksButton('Close');

    await when.userOpensDeleteSiteDialog('Mountain View MTV');
    await then.textIsVisible('Admin access required');
  });

  it('renders an error alert when ListPhysicalSites fails', async () => {
    given.listSitesError('Database connection unavailable');

    when.pageIsRendered();

    await then.textIsVisible(/Database connection unavailable/);
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
