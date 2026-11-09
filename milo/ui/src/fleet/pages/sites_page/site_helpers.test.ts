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

import {
  PhysicalSite,
  Platform,
  PlatformDeviceFilter,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  buildPlatformFilters,
  computeSitePlatformRollup,
  filterAndSortSites,
  formatHealthyPercentage,
  formatSiteUpdatedAt,
  getCampusMapsUrl,
  getPlatformDevicesUrl,
  getSnipItShortlinkLabel,
  normalizeSiteImageUrl,
  parseSiteImageUrlsInput,
  platformFiltersToMap,
} from './site_helpers';

describe('site_helpers', () => {
  it('normalizes Campus Maps URLs to 3-segment building POI selection links', () => {
    expect(getCampusMapsUrl('US-PAO-EM25')).toBe(
      'https://goto.google.com/where/?id=US-PAO-EM25',
    );
    expect(getCampusMapsUrl('US-PAO-EM25-1')).toBe(
      'https://goto.google.com/where/?id=US-PAO-EM25',
    );
    expect(getCampusMapsUrl('IN-BLR-BCPC-9-9009')).toBe(
      'https://goto.google.com/where/?id=IN-BLR-BCPC',
    );
    expect(getCampusMapsUrl('EM25')).toBe(
      'https://goto.google.com/where/?id=EM25',
    );
    expect(getCampusMapsUrl('   ')).toBeNull();
  });

  it('formats updatedAt timestamps to ISO YYYY-MM-DD', () => {
    expect(formatSiteUpdatedAt('2026-11-05T20:15:00Z')).toBe('2026-11-05');
    expect(formatSiteUpdatedAt(undefined)).toBeNull();
    expect(formatSiteUpdatedAt('invalid-date')).toBeNull();
  });

  it('builds platform-specific device list URLs with encoded filters', () => {
    expect(
      getPlatformDevicesUrl(Platform.CHROMEOS, 'labels.zone = "ZONE_SFO36_OS"'),
    ).toBe(
      '/ui/fleet/p/chromeos/devices?filters=labels.zone+%3D+%22ZONE_SFO36_OS%22',
    );
    expect(
      getPlatformDevicesUrl(Platform.ANDROID, 'lab_name = "sjc-mdpt9"'),
    ).toBe('/ui/fleet/p/android/devices?filters=lab_name+%3D+%22sjc-mdpt9%22');
    expect(
      getPlatformDevicesUrl(Platform.CHROMIUM, 'ufs_zone = "SFO36_BROWSER"'),
    ).toBe(
      '/ui/fleet/p/chromium/devices?filters=ufs_zone+%3D+%22SFO36_BROWSER%22',
    );
    expect(getPlatformDevicesUrl(Platform.CHROMEOS, '   ')).toBeNull();
  });

  it('computes multi-platform device rollup totals, share percentages, and weighted health', () => {
    const rollup = computeSitePlatformRollup([
      {
        platform: Platform.CHROMEOS,
        filter: 'labels.zone = "ZONE_SFO36_OS"',
        total: 300,
        healthyCount: 270,
        isLoading: false,
        isError: false,
      },
      {
        platform: Platform.ANDROID,
        filter: 'lab_name = "sjc-mdpt9"',
        total: 100,
        healthyCount: 70,
        isLoading: false,
        isError: false,
      },
      {
        platform: Platform.CHROMIUM,
        filter: 'ufs_zone = "SFO36_BROWSER"',
        total: 100,
        healthyCount: null,
        isHealthTbd: true,
        isLoading: false,
        isError: false,
      },
    ]);

    expect(rollup.totalDevices).toBe(500);
    // Weighted across ChromeOS (270/300) + Android (70/100) = 340/400 = 85.0%
    expect(rollup.overallHealthyPercent).toBe(85);
    expect(formatHealthyPercentage(rollup.overallHealthyPercent!)).toBe(
      '85.0%',
    );
    expect(rollup.isAllHealthTbd).toBe(false);
    expect(
      rollup.rows.map((r) => [r.label, r.sharePercent, r.healthyPercent]),
    ).toEqual([
      ['ChromeOS', 60, 90],
      ['Android', 20, 70],
      ['Browser', 20, null],
    ]);
  });

  it('round-trips non-empty platformFilters in canonical platform order', () => {
    const filters = [
      PlatformDeviceFilter.fromPartial({
        platform: Platform.CHROMIUM,
        filter: ' ufs_zone = "ATL97" ',
      }),
      PlatformDeviceFilter.fromPartial({
        platform: Platform.CHROMEOS,
        filter: ' labels.zone = "ZONE_SFO36_OS" ',
      }),
    ];
    const map = platformFiltersToMap(filters);
    expect(map).toEqual({
      [Platform.CHROMEOS]: 'labels.zone = "ZONE_SFO36_OS"',
      [Platform.CHROMIUM]: 'ufs_zone = "ATL97"',
    });
    expect(buildPlatformFilters(map)).toEqual([
      PlatformDeviceFilter.fromPartial({
        platform: Platform.CHROMEOS,
        filter: 'labels.zone = "ZONE_SFO36_OS"',
      }),
      PlatformDeviceFilter.fromPartial({
        platform: Platform.CHROMIUM,
        filter: 'ufs_zone = "ATL97"',
      }),
    ]);
  });

  it('filters sites by platform filter and fuzzy query', () => {
    const sites = [
      PhysicalSite.fromPartial({
        id: 'EM25',
        name: 'Palo Alto EM25',
        platformFilters: [
          { platform: Platform.CHROMEOS, filter: 'labels.zone = "EM25"' },
        ],
      }),
      PhysicalSite.fromPartial({
        id: 'MTV',
        name: 'Mountain View MTV',
        platformFilters: [
          { platform: Platform.ANDROID, filter: 'lab_name = "mtv"' },
        ],
      }),
    ];
    expect(
      filterAndSortSites(sites, '', Platform.CHROMEOS).map((s) => s.id),
    ).toEqual(['EM25']);
    expect(
      filterAndSortSites(sites, '', Platform.ANDROID).map((s) => s.id),
    ).toEqual(['MTV']);
  });

  it('normalizes SnipIt shortlinks and URLs for facility photos', () => {
    expect(normalizeSiteImageUrl('screen/ddNrsT8xfFq4nJa')).toBe(
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
    );
    expect(normalizeSiteImageUrl('screen/ddNrsT8xfFq4nJa/')).toBe(
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
    );
    expect(normalizeSiteImageUrl('http://screen/ddNrsT8xfFq4nJa/')).toBe(
      'https://screenshot.googleplex.com/ddNrsT8xfFq4nJa.png',
    );
    expect(
      normalizeSiteImageUrl('https://screenshot.googleplex.com/abc123_XY'),
    ).toBe('https://screenshot.googleplex.com/abc123_XY.png');
    expect(normalizeSiteImageUrl('not-a-url')).toBeNull();
    expect(
      parseSiteImageUrlsInput('screen/imgOne, https://example.com/lab.jpg'),
    ).toEqual([
      'https://screenshot.googleplex.com/imgOne.png',
      'https://example.com/lab.jpg',
    ]);
    expect(
      getSnipItShortlinkLabel('https://screenshot.googleplex.com/imgOne.png'),
    ).toBe('screen/imgOne');
  });
});
