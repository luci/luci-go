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

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

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

  it('renders physical site cards with multi-platform rollup counts, weighted health, device links, and Campus Maps links', async () => {
    given.platformCounts({
      chromeos: { total: 300, ready: 270 },
      android: { totalDevices: 100, inService: 70 },
    });
    given.sites([
      { id: 'MTV', name: 'Mountain View MTV' },
      { id: 'LAB2', name: 'Central Lab' },
      { id: 'LAB1', name: 'Central Lab' },
      {
        id: 'US-PAO-EM25',
        name: 'Palo Alto EM25',
        platformFilters: [
          {
            platform: Platform.CHROMEOS,
            filter: '(labels."zone" = "ZONE_SFO36_OS")',
          },
          {
            platform: Platform.ANDROID,
            filter: '("lab_name" = "sjc-mdpt9")',
          },
        ],
      },
    ]);

    when.pageIsRendered();

    await then.siteCardIsVisible('Central Lab', 'LAB1');
    await then.siteCardIsVisible('Central Lab', 'LAB2');
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');
    await then.siteCardIsVisible('Palo Alto EM25', 'US-PAO-EM25');
    await then.siteCardHasCampusMapsLink(
      'US-PAO-EM25',
      'https://goto.google.com/where/?id=US-PAO-EM25',
    );
    await then.siteCardShowsRollupTotal('US-PAO-EM25', '400', '85.0%');
    await then.siteCardHasLink(
      'US-PAO-EM25',
      'ChromeOS devices',
      '/ui/fleet/p/chromeos/devices?filters=%28labels.%22zone%22+%3D+%22ZONE_SFO36_OS%22%29',
    );
    await then.siteCardHasLink(
      'US-PAO-EM25',
      'Android devices',
      '/ui/fleet/p/android/devices?filters=%28%22lab_name%22+%3D+%22sjc-mdpt9%22%29',
    );
    await then.siteCardIdOrderIs(['LAB1', 'LAB2', 'MTV', 'US-PAO-EM25']);
    await then.textIsVisible('4 sites');
  });

  it('filters physical site cards by name, building code, or platform chip and supports clearing the filter', async () => {
    given.sites([
      {
        id: 'MTV',
        name: 'Mountain View MTV',
        platformFilters: [
          { platform: Platform.ANDROID, filter: '("lab_name" = "mtv")' },
        ],
      },
      {
        id: 'EM25',
        name: 'Palo Alto EM25',
        platformFilters: [
          {
            platform: Platform.CHROMEOS,
            filter: '(labels."zone" = "ZONE_SFO36_OS")',
          },
        ],
      },
    ]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');

    await when.userSelectsPlatformFilter('ChromeOS');
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');
    then.siteCardIsNotVisible('Mountain View MTV');

    await when.userSelectsPlatformFilter('Platform: All');
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

  it('creates a new physical site with platform filters via the unified FilterBar and refreshes the sorted card grid', async () => {
    given.sites([{ id: 'MTV', name: 'Mountain View MTV' }]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Mountain View MTV', 'MTV');

    await when.userOpensAddSiteDialog();
    await when.userFillsSiteForm({
      id: 'EM25',
      name: 'Palo Alto EM25',
      chromeosFilter: 'labels.zone:ZONE_SFO36_OS',
      snipItPhotoLink: 'screen/ddNrsT8xfFq4nJa',
    });
    await when.userSubmitsSiteForm();

    then.rpcWasCalledWith('CreatePhysicalSite', {
      physicalSite: {
        id: 'EM25',
        name: 'Palo Alto EM25',
        platformFilters: [
          {
            platform: 'CHROMEOS',
            filter: '(labels."zone" = "ZONE_SFO36_OS")',
          },
        ],
        imageUrls: ['https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png'],
      },
    });
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');
    await then.siteCardOrderIs(['Mountain View MTV', 'Palo Alto EM25']);
    await then.siteCardShowsFacilityPhoto(
      'EM25',
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
    );
  });

  it('navigates across multiple facility photos in the site card carousel', async () => {
    given.sites([
      {
        id: 'EM25',
        name: 'Palo Alto EM25',
        imageUrls: [
          'screen/ddNrsT8xfFq4nJa',
          'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
          'https://example.com/lab-2.png',
        ],
      },
    ]);

    when.pageIsRendered();

    await then.siteCardShowsPhotoBadge('EM25', '1 / 2');
    await then.siteCardPreloadsAllFacilityPhotos('EM25', [
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
      'https://example.com/lab-2.png',
    ]);
    await then.siteCardShowsFacilityPhoto(
      'EM25',
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
    );

    await when.userClicksButton('Next photo for Palo Alto EM25');
    await then.siteCardShowsPhotoBadge('EM25', '2 / 2');
    await then.siteCardShowsFacilityPhoto(
      'EM25',
      'https://example.com/lab-2.png',
    );

    await when.userClicksButton('Previous photo for Palo Alto EM25');
    await then.siteCardShowsPhotoBadge('EM25', '1 / 2');
    await then.siteCardPreloadsAllFacilityPhotos('EM25', [
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
      'https://example.com/lab-2.png',
    ]);
    await then.siteCardShowsFacilityPhoto(
      'EM25',
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
    );
  });

  it('displays the facility photo placeholder when no photos exist or an image fails to load', async () => {
    given.sites([
      { id: 'MTV', name: 'Mountain View MTV' },
      {
        id: 'EM25',
        name: 'Palo Alto EM25',
        imageUrls: ['https://example.com/broken.png'],
      },
    ]);

    when.pageIsRendered();

    await then.siteCardShowsPhotoPlaceholder('MTV');
    await then.siteCardShowsFacilityPhoto(
      'EM25',
      'https://example.com/broken.png',
    );

    await when.facilityPhotoFailsToLoad('EM25');

    await then.siteCardShowsPhotoPlaceholder('EM25');
  });

  it('adds, removes, and validates facility photo links in the site form dialog', async () => {
    given.sites([
      {
        id: 'EM25',
        name: 'Palo Alto EM25',
        imageUrls: [
          'screen/ddNrsT8xfFq4nJa',
          'https://example.com/keep-lab.png',
        ],
      },
    ]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Palo Alto EM25', 'EM25');

    await when.userOpensEditSiteDialog('Palo Alto EM25');
    await when.userRemovesPhotoInDialog('screen/ddNrsT8xfFq4nJa');

    await when.userTypesPhotoInput('screen/YLwMYTzGAfHLEXf, invalid-token');
    await then.textIsVisible(
      'Enter a SnipIt link (screen/ID) or https:// image URL.',
    );
    await then.dialogAddPhotoButtonIsDisabled();
    await then.siteFormSubmitIsDisabled();

    await when.userTypesPhotoInput('screen/YLwMYTzGAfHLEXf/');
    await when.userClicksAddPhotoInDialog();
    await when.userSubmitsSiteForm();

    then.rpcWasCalledWith('UpdatePhysicalSite', {
      physicalSite: {
        id: 'EM25',
        name: 'Palo Alto EM25',
        imageUrls: [
          'https://example.com/keep-lab.png',
          'https://screenshot.googleplex.com/YLwMYTzGAfHLEXf.png',
        ],
      },
    });
  });

  it('opens the edit dialog from Add filters on an unconfigured card and saves updated filters', async () => {
    given.sites([{ id: 'EM25', name: 'Palo Alto' }]);

    when.pageIsRendered();
    await then.siteCardIsVisible('Palo Alto', 'EM25');

    await when.userClicksAddFiltersOnCard('EM25');
    await then.siteIdInputIsDisabled();
    await when.userFillsSiteForm({
      name: 'Palo Alto EM25',
      androidFilter: 'lab_name:sjc-mdpt9',
    });
    await when.userSubmitsSiteForm();

    then.rpcWasCalledWith('UpdatePhysicalSite', {
      physicalSite: {
        id: 'EM25',
        name: 'Palo Alto EM25',
        platformFilters: [
          { platform: 'ANDROID', filter: '("lab_name" = "sjc-mdpt9")' },
        ],
      },
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
