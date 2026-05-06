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

import { RRI_VERSION_TEST_URLS } from '@/fleet/testing_tools/version_test_urls';

import { detectVersion, mapUrl, VERSION_PARAM_KEY } from './rri_url_migration';

describe('rri_url_migration', () => {
  describe('detectVersion', () => {
    it('should return 1 if version is missing', () => {
      const params = new URLSearchParams();
      expect(detectVersion(params)).toBe(1);
    });

    it('should return the version if present', () => {
      const params = new URLSearchParams();
      params.set(VERSION_PARAM_KEY, '2');
      expect(detectVersion(params)).toBe(2);
    });

    it('should return 1 if version is invalid', () => {
      const params = new URLSearchParams();
      params.set(VERSION_PARAM_KEY, 'abc');
      expect(detectVersion(params)).toBe(1);
    });
  });

  describe('mapUrl', () => {
    it('should map from v1 to v2', () => {
      const params = new URLSearchParams();
      // Assuming v1 has no version param
      const { migratedParams, needsMigration } = mapUrl(params);
      expect(needsMigration).toBe(true);
      expect(migratedParams.get(VERSION_PARAM_KEY)).toBe('2');
    });

    it('should convert legacy filters to AIP-160', () => {
      const params = new URLSearchParams();
      const legacyFilters =
        'rr_id=("NPI-10108" OR "NPI-10108-1")&' +
        'fulfillment_status="NOT_STARTED"&' +
        'resource_request_target_delivery_date_min=2025-01-01&' +
        'resource_request_target_delivery_date_max=2026-01-01';
      params.set('filters', legacyFilters);

      const { migratedParams, needsMigration } = mapUrl(params);

      expect(needsMigration).toBe(true);
      expect(migratedParams.get(VERSION_PARAM_KEY)).toBe('2');

      const aip160 = migratedParams.get('filters');
      expect(aip160).toContain(
        '(rr_id = "NPI-10108" OR rr_id = "NPI-10108-1")',
      );
      expect(aip160).toContain('fulfillment_status = "NOT_STARTED"');
      expect(aip160).toContain(
        'resource_request_target_delivery_date >= "2025-01-01"',
      );
      expect(aip160).toContain(
        'resource_request_target_delivery_date <= "2026-01-01"',
      );
    });

    it('should not map if already at latest version', () => {
      const params = new URLSearchParams();
      params.set(VERSION_PARAM_KEY, '2');
      const { migratedParams, needsMigration } = mapUrl(params);
      expect(needsMigration).toBe(false);
      expect(migratedParams.get(VERSION_PARAM_KEY)).toBe('2');
    });

    const migrationTestCases = [
      {
        name: 'map full legacy URL with new values to v2',
        inputUrl: RRI_VERSION_TEST_URLS.v1,
        expectedVersion: '2',
        expectedFiltersContain: [
          '(rr_id = "RR-001" OR rr_id = "RR-0012")',
          '(resource_details = "FF Creation Testing - [Resource Request Demo][Annual]: ' +
            'Pixel Tablet (2023) x 549 units for xTS Quality Improvement - Devices for automated testing" OR ' +
            'resource_details = "FF Creation Testing 2 - [Resource Request Demo][Annual]: ' +
            'Pixel Tablet (2023) x 451 units for xTS Quality Improvement - Devices for automated testing")',
          'fulfillment_status = "NOT_STARTED"',
          'resource_request_target_delivery_date >= "2025-01-01"',
          'resource_request_target_delivery_date <= "2026-01-01"',
          'material_sourcing_actual_delivery_date >= "2026-01-01"',
          'build_actual_delivery_date <= "2026-01-01"',
          '(customer = "ANPLAT" OR customer = "BROTHER")',
          '(resource_groups = "ANDROID_TABLET" OR resource_groups = "CHROME_DESKTOP")',
        ],
      },
    ];

    migrationTestCases.forEach((tc) => {
      it(`should ${tc.name}`, () => {
        const queryString = tc.inputUrl.split('?')[1];
        const params = new URLSearchParams(queryString);

        const { migratedParams, needsMigration } = mapUrl(params);

        expect(needsMigration).toBe(true);
        expect(migratedParams.get(VERSION_PARAM_KEY)).toBe(tc.expectedVersion);

        const aip160 = migratedParams.get('filters');
        tc.expectedFiltersContain.forEach((part) => {
          expect(aip160).toContain(part);
        });
      });
    });
  });

  //old version: https://ci.chromium.org/ui/fleet/requests?filters=rr_id%3D%28%22NPI-10108%22+OR+%22NPI-10108-1%22%29%26resource_details%3D%28%22FF+Creation+Testing+2+-+%5BResource+Request+Demo%5D%5BAnnual%5D%3A+Pixel+Tablet+%282023%29+x+451+units+for+xTS+Quality+Improvement+-+Devices+for+automated+testing%22+OR+%22Infrastructure+NPI+Request%3A+%5B+RF+shield+enclosure+rack+solution+to+expand+wifi+test+for+Browser+%5D%22%29%26fulfillment_status%3D%22NOT_STARTED%22%26resource_request_target_delivery_date_min%3D2025-01-01%26resource_request_target_delivery_date_max%3D2026-01-01%26material_sourcing_actual_delivery_date_min%3D2026-01-01%26build_actual_delivery_date_max%3D2026-01-01%26customer%3D%28%22ANMAIN%22+OR+%22ANOTHER%22%29%26resource_groups%3D%28%22ANDROID_TABLET%22+OR+%22BETO_TESTBED%22+OR+%22CHROMEOS_DUT%22%29
  //new version: https://ci.chromium.org/ui/fleet/requests?v=2&filters=%28rr_id+%3D+%22RR-001%22+OR+rr_id+%3D+%22RR-0012%22%29+AND+%28resource_details+%3D+%22FF+Creation+Testing+-+%5BResource+Request+Demo%5D%5BAnnual%5D%3A+Pixel+Tablet+%282023%29+x+549+units+for+xTS+Quality+Improvement+-+Devices+for+automated+testing%22+OR+resource_details+%3D+%22FF+Creation+Testing+2+-+%5BResource+Request+Demo%5D%5BAnnual%5D%3A+Pixel+Tablet+%282023%29+x+451+units+for+xTS+Quality+Improvement+-+Devices+for+automated+testing%22%29+AND+%28fulfillment_status+%3D+NOT_STARTED%29+AND+resource_request_target_delivery_date+%3E%3D+%222025-01-01%22+AND+resource_request_target_delivery_date+%3C%3D+%222026-01-01%22+AND+material_sourcing_actual_delivery_date+%3E%3D+%222026-01-01%22+AND+build_actual_delivery_date+%3C%3D+%222026-01-01%22+AND+%28customer+%3D+%22ANPLAT%22+OR+customer+%3D+%22BROTHER%22%29+AND+%28resource_groups+%3A+%22ANDROID_TABLET%22+OR+resource_groups+%3A+%22CHROME_DESKTOP%22%29
});
