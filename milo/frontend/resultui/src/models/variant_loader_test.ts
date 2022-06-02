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
import sinon from 'sinon';

import { CacheOption } from '../libs/cached_fn';
import { QueryVariantsRequest, QueryVariantsResponse, TestHistoryService } from '../services/weetbix';
import { VariantLoader } from './variant_loader';

const utv1 = {
  variantHash: 'key1:val1',
  variant: {
    def: {
      key1: 'val1',
    },
  },
};

const utv2 = {
  variantHash: 'key1:val2',
  variant: {
    def: {
      key1: 'val2',
    },
  },
};

describe('TestHistoryLoader', () => {
  it('discoverVariants should work correctly', async () => {
    // Set up.
    const queryVariantsStub = sinon.stub<[QueryVariantsRequest, CacheOption], Promise<QueryVariantsResponse>>();
    const variantLoader = new VariantLoader('project', 'realm', 'test-id', { contains: { def: {} } }, {
      queryVariants: queryVariantsStub,
    } as Partial<TestHistoryService> as TestHistoryService);

    // Before loading.
    assert.strictEqual(variantLoader.variants.length, 0);

    // Discover variants from the first page.
    queryVariantsStub.onCall(0).resolves({ variants: [utv1], nextPageToken: 'page2' });
    let done = await variantLoader.discoverVariants();
    assert.isFalse(done);
    assert.deepEqual(queryVariantsStub.getCalls().length, 1);
    assert.deepIncludeProperties(queryVariantsStub.getCall(0).args[0], {
      project: 'project',
      subRealm: 'realm',
      testId: 'test-id',
      pageToken: '',
    });

    // After loading.
    assert.deepEqual(variantLoader.variants, [['key1:val1', { def: { key1: 'val1' } }]]);

    // Discover variants from the second page.
    queryVariantsStub.onCall(1).resolves({ variants: [utv2] });
    done = await variantLoader.discoverVariants();
    assert.isTrue(done);
    assert.deepEqual(queryVariantsStub.getCalls().length, 2);
    assert.deepIncludeProperties(queryVariantsStub.getCall(1).args[0], {
      project: 'project',
      subRealm: 'realm',
      testId: 'test-id',
      pageToken: 'page2',
    });

    // After loading.
    assert.deepEqual(variantLoader.variants, [
      ['key1:val1', { def: { key1: 'val1' } }],
      ['key1:val2', { def: { key1: 'val2' } }],
    ]);
  });
});
