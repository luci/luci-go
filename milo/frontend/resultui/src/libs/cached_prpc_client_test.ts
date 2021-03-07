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

import { PrpcClient } from '@chopsui/prpc-client';
import { assert } from 'chai';
import * as sinon from 'sinon';

import { CachedPrpcClient } from './cached_prpc_client';

describe('cached_prpc_client', () => {
  let client: CachedPrpcClient;
  let callStub: sinon.SinonStub<
    [
      service: string,
      method: string,
      message: object,
      additionalHeaders?: {[key: string]: string}
    ],
    Promise<unknown>
  >;
  beforeEach(() => {
    const prpcClient = new PrpcClient();
    callStub = sinon.stub(prpcClient, 'call');
    callStub.onCall(0).resolves('response 0');
    callStub.onCall(1).resolves('response 1');
    client = new CachedPrpcClient(prpcClient);
  });

  it('should return cached response when calls are identical', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val2'});
    const res2 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val2'});
    assert.strictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 1);
  });

  it('should treat empty additionalHeaders and undefined additionalHeaders the same', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, undefined);
    const res2 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {});
    assert.strictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 1);
  });

  it('should not return cached response when services are different', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, undefined);
    const res2 = await client.call('service_b', 'method_b', {'key1': 'val1'}, {});
    assert.notStrictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 2);
  });

  it('should not return cached response when methods are different', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, undefined);
    const res2 = await client.call('service_a', 'method_c', {'key1': 'val1'}, {});
    assert.notStrictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 2);
  });

  it('should not return cached response when messages are different', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, undefined);
    const res2 = await client.call('service_a', 'method_b', {'key1': 'val2'}, {});
    assert.notStrictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 2);
  });

  it('should not return cached response when headers are different', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'});
    const res2 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val2'}, true);
    assert.notStrictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 2);
  });

  it('should not return cached response when forceRefresh is set to true', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'});
    const res2 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'}, true);
    assert.notStrictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 2);
  });

  it('should cache response when forceRefresh is set to true', async () => {
    const res1 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'}, true);
    // When forceRefresh is set to true, cached result is not used,
    // but the cache is still refreshed.
    const res2 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'}, true);
    const res3 = await client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'}, false);
    assert.notStrictEqual(res1, res2);
    assert.strictEqual(res2, res3);
    assert.strictEqual(callStub.callCount, 2);
  });

  it('should handle concurrent calls correctly', async () => {
    const req1 = client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'});
    const req2 = client.call('service_a', 'method_b', {'key1': 'val1'}, {'header_key1': 'header_val1'});
    const res2 = await req2;
    const res1 = await req1;
    assert.strictEqual(res1, res2);
    assert.strictEqual(callStub.callCount, 1);
  });
});
