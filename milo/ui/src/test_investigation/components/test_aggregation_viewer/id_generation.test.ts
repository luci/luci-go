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
  AggregationLevel,
  TestIdentifier,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { createPrefixForTest, getTestIdentifierPrefixId } from './utils';

describe('ID Generation Consistency', () => {
  const fullTestId: TestIdentifier = {
    moduleName: 'test_module',
    moduleScheme: 'junit',
    coarseName: 'coarse_pkg',
    fineName: 'FineClass',
    caseName: 'methodName',
    moduleVariant: { def: {} },
    moduleVariantHash: '',
  };

  it('should generate clean Module ID from full TestIdentifier', () => {
    // Mimic processVerdict logic
    const modulePrefix = createPrefixForTest(
      { testIdStructured: fullTestId } as TestVariant,
      AggregationLevel.MODULE,
    );
    const id = getTestIdentifierPrefixId(modulePrefix);

    // Should NOT contain CRS or FNE or CAS
    expect(id).toContain('MOD:test_module');
    expect(id).not.toContain('CRS:');
    expect(id).not.toContain('FNE:');
    expect(id).not.toContain('CAS:');
    expect(id).toBe('LEVEL:2|MOD:test_module|SCH:junit');
  });

  it('should generate clean Coarse ID from full TestIdentifier', () => {
    const coarsePrefix = createPrefixForTest(
      { testIdStructured: fullTestId } as TestVariant,
      AggregationLevel.COARSE,
    );
    const id = getTestIdentifierPrefixId(coarsePrefix);

    expect(id).toContain('MOD:test_module');
    expect(id).toContain('CRS:coarse_pkg');
    expect(id).not.toContain('FNE:');
    expect(id).toBe('LEVEL:3|MOD:test_module|SCH:junit|CRS:coarse_pkg');
  });
});
