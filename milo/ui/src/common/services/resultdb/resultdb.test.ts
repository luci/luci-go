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

import {
  createTVCmpFn,
  createTVPropGetter,
  getInvIdFromBuildId,
  getInvIdFromBuildNum,
  TestVariant,
  TestVerdict_Status,
  TestVerdict_StatusOverride,
} from './resultdb';

describe('resultdb', () => {
  test('should compute invocation ID from build number correctly', async () => {
    const invId = await getInvIdFromBuildNum(
      { project: 'chromium', bucket: 'ci', builder: 'ios-device' },
      179945,
    );
    expect(invId).toStrictEqual(
      'build-135d246ed1a40cc3e77d8b1daacc7198fe344b1ac7b95c08cb12f1cc383867d7-179945',
    );
  });

  test('should compute invocation ID from build ID correctly', async () => {
    const invId = await getInvIdFromBuildId('123456');
    expect(invId).toStrictEqual('build-123456');
  });
});

describe('createTVPropGetter', () => {
  test('can create a status getter', async () => {
    const getter = createTVPropGetter('status');
    const prop1 = getter({
      statusV2: TestVerdict_Status.FAILED,
      statusOverride: TestVerdict_StatusOverride.EXONERATED,
    } as Partial<TestVariant> as TestVariant);
    expect(prop1).toStrictEqual(TestVerdict_StatusOverride.EXONERATED);

    const prop2 = getter({
      statusV2: TestVerdict_Status.FAILED,
      statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
    } as Partial<TestVariant> as TestVariant);
    expect(prop2).toStrictEqual(TestVerdict_Status.FAILED);
  });

  test('can create a name getter', async () => {
    const getter = createTVPropGetter('Name');

    const prop1 = getter({
      testId: 'test-id',
      testMetadata: { name: 'test-name' },
    } as Partial<TestVariant> as TestVariant);
    expect(prop1).toStrictEqual('test-name');

    // Fallback to test id.
    const prop2 = getter({
      testId: 'test-id',
    } as Partial<TestVariant> as TestVariant);
    expect(prop2).toStrictEqual('test-id');
  });

  test('can create a variant value getter', async () => {
    const getter = createTVPropGetter('v.variant_key');

    const prop1 = getter({
      variant: { def: { variant_key: 'variant_value' } },
    } as Partial<TestVariant> as TestVariant);
    expect(prop1).toStrictEqual('variant_value');

    // Fallback to empty string.
    const prop2 = getter({} as Partial<TestVariant> as TestVariant);
    expect(prop2).toStrictEqual('');
  });
});

describe('createTVCmpFn', () => {
  const variant1 = {
    statusV2: TestVerdict_Status.FAILED,
    statusOverride: TestVerdict_StatusOverride.NOT_OVERRIDDEN,
    testMetadata: {
      name: 'a',
    },
    variant: { def: { key1: 'val1' } },
  } as Partial<TestVariant> as TestVariant;
  const variant2 = {
    statusV2: TestVerdict_Status.FAILED,
    statusOverride: TestVerdict_StatusOverride.EXONERATED,
    testMetadata: {
      name: 'b',
    },
    variant: { def: { key1: 'val2' } },
  } as Partial<TestVariant> as TestVariant;
  const variant3 = {
    statusV2: TestVerdict_Status.EXECUTION_ERRORED,
    statusOverride: TestVerdict_StatusOverride.EXONERATED,
    testMetadata: {
      name: 'b',
    },
    variant: { def: { key1: 'val1' } },
  } as Partial<TestVariant> as TestVariant;

  test('can create a sort fn', async () => {
    const cmpFn = createTVCmpFn(['name']);

    expect(cmpFn(variant1, variant2)).toStrictEqual(-1);
    expect(cmpFn(variant2, variant1)).toStrictEqual(1);
    expect(cmpFn(variant1, variant1)).toStrictEqual(0);
  });

  test('can sort in descending order', async () => {
    const cmpFn = createTVCmpFn(['-name']);

    expect(cmpFn(variant1, variant2)).toStrictEqual(1);
    expect(cmpFn(variant2, variant1)).toStrictEqual(-1);
    expect(cmpFn(variant1, variant1)).toStrictEqual(0);
  });

  test('can sort by status correctly', async () => {
    const cmpFn = createTVCmpFn(['status']);

    // Status should be treated as numbers rather than as strings when sorting.
    expect(cmpFn(variant1, variant2)).toStrictEqual(-1);
    expect(cmpFn(variant2, variant1)).toStrictEqual(1);
    expect(cmpFn(variant1, variant1)).toStrictEqual(0);

    expect(cmpFn(variant1, variant3)).toStrictEqual(-1);
    expect(cmpFn(variant3, variant1)).toStrictEqual(1);
    expect(cmpFn(variant2, variant3)).toStrictEqual(0);
  });

  test('can sort by multiple keys', async () => {
    const cmpFn = createTVCmpFn(['status', '-v.key1']);

    expect(cmpFn(variant1, variant2)).toStrictEqual(-1);
    expect(cmpFn(variant2, variant1)).toStrictEqual(1);
    expect(cmpFn(variant2, variant3)).toStrictEqual(-1);
  });
});
