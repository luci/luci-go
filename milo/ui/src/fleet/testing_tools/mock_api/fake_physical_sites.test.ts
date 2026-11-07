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
} from '../../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { createPhysicalSitesFake } from './fake_physical_sites';

describe('createPhysicalSitesFake', () => {
  it('lists seeded sites ordered deterministically by name then id', async () => {
    const fake = createPhysicalSitesFake();
    fake.seed([
      { id: ' MTV2 ', name: ' Mountain View ' },
      { id: 'LAB1', name: 'Central Lab' },
      { id: 'EM25', name: 'Palo Alto' },
      { id: 'MTV1', name: 'Mountain View' },
    ]);

    const res = await fake.ListPhysicalSites({});
    expect(res.physicalSites).toEqual([
      PhysicalSite.fromPartial({ id: 'LAB1', name: 'Central Lab' }),
      PhysicalSite.fromPartial({ id: 'MTV1', name: 'Mountain View' }),
      PhysicalSite.fromPartial({ id: 'MTV2', name: 'Mountain View' }),
      PhysicalSite.fromPartial({ id: 'EM25', name: 'Palo Alto' }),
    ]);
  });

  it('supports creating, updating, and deleting physical sites with V1.1 fields', async () => {
    const fake = createPhysicalSitesFake();

    const missingUpdate = await fake.UpdatePhysicalSite({
      physicalSite: PhysicalSite.fromPartial({
        id: 'UNKNOWN',
        name: 'Ignored',
      }),
    });
    expect(missingUpdate.physicalSite).toBeUndefined();

    const created = await fake.CreatePhysicalSite({
      physicalSite: PhysicalSite.fromPartial({
        id: 'EM25',
        name: 'Palo Alto',
        imageUrls: ['https://screenshot.googleplex.com/abc.png'],
        platformFilters: [
          {
            platform: Platform.CHROMEOS,
            filter: 'labels.zone = "ZONE_SFO36_OS"',
          },
        ],
      }),
    });
    expect(created.physicalSite).toEqual(
      PhysicalSite.fromPartial({
        id: 'EM25',
        name: 'Palo Alto',
        imageUrls: ['https://screenshot.googleplex.com/abc.png'],
        platformFilters: [
          {
            platform: Platform.CHROMEOS,
            filter: 'labels.zone = "ZONE_SFO36_OS"',
          },
        ],
      }),
    );

    const updated = await fake.UpdatePhysicalSite({
      physicalSite: PhysicalSite.fromPartial({
        id: 'EM25',
        name: 'Palo Alto EM25',
        imageUrls: ['https://screenshot.googleplex.com/updated.png'],
        platformFilters: [
          {
            platform: Platform.CHROMEOS,
            filter: 'labels.zone = "ZONE_SFO36_OS"',
          },
        ],
      }),
    });
    expect(updated.physicalSite).toEqual(
      PhysicalSite.fromPartial({
        id: 'EM25',
        name: 'Palo Alto EM25',
        imageUrls: ['https://screenshot.googleplex.com/updated.png'],
        platformFilters: [
          {
            platform: Platform.CHROMEOS,
            filter: 'labels.zone = "ZONE_SFO36_OS"',
          },
        ],
      }),
    );

    const listedAfterUpdate = await fake.ListPhysicalSites({});
    expect(listedAfterUpdate.physicalSites).toEqual([updated.physicalSite]);

    await fake.DeletePhysicalSite({ id: ' EM25 ' });
    const listedAfterDelete = await fake.ListPhysicalSites({});
    expect(listedAfterDelete.physicalSites).toEqual([]);
  });
});
