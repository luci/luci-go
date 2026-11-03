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
      { id: 'LAB1', name: 'Central Lab' },
      { id: 'MTV1', name: 'Mountain View' },
      { id: 'MTV2', name: 'Mountain View' },
      { id: 'EM25', name: 'Palo Alto' },
    ]);
  });

  it('supports creating, updating, and deleting physical sites', async () => {
    const fake = createPhysicalSitesFake();

    const missingUpdate = await fake.UpdatePhysicalSite({
      physicalSite: { id: 'UNKNOWN', name: 'Ignored' },
    });
    expect(missingUpdate.physicalSite).toBeUndefined();

    const created = await fake.CreatePhysicalSite({
      physicalSite: { id: 'EM25', name: 'Palo Alto' },
    });
    expect(created.physicalSite).toEqual({ id: 'EM25', name: 'Palo Alto' });

    const updated = await fake.UpdatePhysicalSite({
      physicalSite: { id: 'EM25', name: 'Palo Alto EM25' },
    });
    expect(updated.physicalSite).toEqual({
      id: 'EM25',
      name: 'Palo Alto EM25',
    });

    const listedAfterUpdate = await fake.ListPhysicalSites({});
    expect(listedAfterUpdate.physicalSites).toEqual([
      { id: 'EM25', name: 'Palo Alto EM25' },
    ]);

    await fake.DeletePhysicalSite({ id: ' EM25 ' });
    const listedAfterDelete = await fake.ListPhysicalSites({});
    expect(listedAfterDelete.physicalSites).toEqual([]);
  });
});
