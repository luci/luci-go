// Copyright 2021 The LUCI Authors.
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

import { assert } from 'chai';

import {
  createTVPropGetter,
  getInvIdFromBuildId,
  getInvIdFromBuildNum,
  TestVariant,
  TestVariantStatus,
} from './resultdb';

describe('resultdb', () => {
  it('should compute invocation ID from build number correctly', async () => {
    const invId = await getInvIdFromBuildNum({ project: 'chromium', bucket: 'ci', builder: 'ios-device' }, 179945);
    assert.strictEqual(invId, 'build-135d246ed1a40cc3e77d8b1daacc7198fe344b1ac7b95c08cb12f1cc383867d7-179945');
  });

  it('should compute invocation ID from build ID correctly', async () => {
    const invId = await getInvIdFromBuildId('123456');
    assert.strictEqual(invId, 'build-123456');
  });
});

describe('createTVPropGetter', () => {
  it('can create a status getter', async () => {
    const getter = createTVPropGetter('status');
    const prop = getter(({ status: TestVariantStatus.EXONERATED } as Partial<TestVariant>) as TestVariant);
    assert.strictEqual(prop, TestVariantStatus.EXONERATED);
  });

  it('can create a name getter', async () => {
    const getter = createTVPropGetter('Name');

    const prop1 = getter(({
      testId: 'test-id',
      testMetadata: { name: 'test-name' },
    } as Partial<TestVariant>) as TestVariant);
    assert.strictEqual(prop1, 'test-name');

    // Fallback to test id.
    const prop2 = getter(({ testId: 'test-id' } as Partial<TestVariant>) as TestVariant);
    assert.strictEqual(prop2, 'test-id');
  });

  it('can create a variant value getter', async () => {
    const getter = createTVPropGetter('v.variant_key');

    const prop1 = getter(({
      variant: { def: { variant_key: 'variant_value' } },
    } as Partial<TestVariant>) as TestVariant);
    assert.strictEqual(prop1, 'variant_value');

    // Fallback to empty string.
    const prop2 = getter(({} as Partial<TestVariant>) as TestVariant);
    assert.strictEqual(prop2, '');
  });
});
