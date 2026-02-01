// Copyright 2025 The LUCI Authors.
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

import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { getTestVariantURL } from './test_info_utils';

describe('getTestVariantURL', () => {
  const invocationId = 'inv-123';

  it('should generate structured URL when testIdStructured is present', () => {
    const testVariant = TestVariant.fromPartial({
      testIdStructured: {
        moduleName: 'module1',
        moduleScheme: 'scheme1',
        moduleVariantHash: 'hash1',
        coarseName: 'coarse1',
        fineName: 'fine1',
        caseName: 'case1',
      },
    });

    const url = getTestVariantURL(invocationId, testVariant);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-123/modules/module1/schemes/scheme1/variants/hash1/coarse/coarse1/fine/fine1/cases/case1',
    );
  });

  it('should generate structured URL for legacy module', () => {
    const testVariant = TestVariant.fromPartial({
      testIdStructured: {
        moduleName: 'legacy',
        moduleScheme: 'legacy',
        moduleVariantHash: 'hash1',
        caseName: 'case1',
      },
    });

    const url = getTestVariantURL(invocationId, testVariant);
    // Should now use the generic structured path, not the special legacy route
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-123/modules/legacy/schemes/legacy/variants/hash1/cases/case1',
    );
  });

  it('should fallback to legacy URL format if testIdStructured is missing', () => {
    const testVariant = TestVariant.fromPartial({
      testId: 'legacy-test-id',
      variantHash: 'legacy-hash',
    });

    const url = getTestVariantURL(invocationId, testVariant);
    expect(url).toBe(
      '/ui/test-investigate/invocations/inv-123/modules/legacy/schemes/legacy/variants/legacy-hash/cases/legacy-test-id',
    );
  });
});
