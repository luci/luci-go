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
  TestIdentifierPrefix,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

import { getTestIdentifierPrefixId } from './utils';

describe('getTestIdentifierPrefixId', () => {
  it('should include moduleVariantHash in the ID if present', () => {
    const prefix = TestIdentifierPrefix.fromPartial({
      level: AggregationLevel.MODULE,
      id: {
        moduleName: 'test_module',
        moduleScheme: 'test_scheme',
        moduleVariantHash: 'deadbeef',
        moduleVariant: {
          def: {
            b: '2',
            a: '1',
          },
        },
      },
    });
    const id = getTestIdentifierPrefixId(prefix);
    expect(id).toContain('VAR:deadbeef');
  });

  it('should produce identical IDs for same variants regardless of object key order (if hash is same)', () => {
    const prefix1 = TestIdentifierPrefix.fromPartial({
      level: AggregationLevel.MODULE,
      id: {
        moduleVariantHash: 'vhash123',
        moduleVariant: {
          def: { a: '1', b: '2' },
        },
      },
    });
    const prefix2 = TestIdentifierPrefix.fromPartial({
      level: AggregationLevel.MODULE,
      id: {
        moduleVariantHash: 'vhash123',
        moduleVariant: {
          def: { b: '2', a: '1' }, // Different insertion order
        },
      },
    });

    expect(getTestIdentifierPrefixId(prefix1)).toBe(
      getTestIdentifierPrefixId(prefix2),
    );
  });
});
