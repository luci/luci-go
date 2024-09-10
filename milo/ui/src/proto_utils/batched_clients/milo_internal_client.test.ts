// Copyright 2024 The LUCI Authors.
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

/* eslint-disable new-cap */

import {
  BatchCheckPermissionsRequest,
  BatchCheckPermissionsResponse,
  MiloInternalClientImpl,
} from '@/proto/go.chromium.org/luci/milo/proto/v1/rpc.pb';

import { BatchedMiloInternalClientImpl } from './milo_internal_client';

describe('BatchedMiloInternalClientImpl', () => {
  let batchCheckPermsSpy: jest.SpiedFunction<
    BatchedMiloInternalClientImpl['BatchCheckPermissions']
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    batchCheckPermsSpy = jest
      .spyOn(MiloInternalClientImpl.prototype, 'BatchCheckPermissions')
      .mockImplementation(async (batchReq) => {
        return BatchCheckPermissionsResponse.fromPartial({
          results: Object.fromEntries(
            batchReq.permissions.map((s) => [s, s.startsWith('allowed-')]),
          ),
        });
      });
  });

  afterEach(() => {
    jest.useRealTimers();
    batchCheckPermsSpy.mockReset();
  });

  it('can batch eligible requests together', async () => {
    const client = new BatchedMiloInternalClientImpl(
      {
        request: jest.fn(),
      },
      { maxBatchSize: 3 },
    );

    const call1 = client.BatchCheckPermissions(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm1',
        permissions: ['allowed-get', 'set'],
      }),
    );
    const call2 = client.BatchCheckPermissions(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm1',
        permissions: ['allowed-get', 'delete'],
      }),
    );
    const call3 = client.BatchCheckPermissions(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm2',
        permissions: ['allowed-get', 'allowed-set'],
      }),
    );

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called only once for each realm.
    expect(batchCheckPermsSpy).toHaveBeenCalledTimes(2);
    expect(batchCheckPermsSpy).toHaveBeenCalledWith(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm1',
        // Duplicated permissions are removed.
        permissions: ['allowed-get', 'set', 'delete'],
      }),
    );
    expect(batchCheckPermsSpy).toHaveBeenCalledWith(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm2',
        permissions: ['allowed-get', 'allowed-set'],
      }),
    );

    // The responses should be just like regular calls.
    expect(await call1).toEqual(
      BatchCheckPermissionsResponse.fromPartial({
        results: {
          'allowed-get': true,
          set: false,
        },
      }),
    );
    expect(await call2).toEqual(
      BatchCheckPermissionsResponse.fromPartial({
        results: {
          'allowed-get': true,
          delete: false,
        },
      }),
    );
    expect(await call3).toEqual(
      BatchCheckPermissionsResponse.fromPartial({
        results: {
          'allowed-get': true,
          'allowed-set': true,
        },
      }),
    );
  });

  it('can handle over batching', async () => {
    const client = new BatchedMiloInternalClientImpl(
      {
        request: jest.fn(),
      },
      { maxBatchSize: 3 },
    );

    const call1 = client.BatchCheckPermissions(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm1',
        permissions: ['allowed-get', 'set'],
      }),
    );
    const call2 = client.BatchCheckPermissions(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm1',
        permissions: ['allowed-get', 'delete'],
      }),
    );
    const call3 = client.BatchCheckPermissions(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm2',
        permissions: ['allowed-get', 'allowed-set'],
      }),
    );
    const call4 = client.BatchCheckPermissions(
      BatchCheckPermissionsRequest.fromPartial({
        realm: 'proj:realm2',
        permissions: ['get', 'set'],
      }),
    );

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called 3 times.
    // 1 time for realm1, 2 times for realm 2.
    expect(batchCheckPermsSpy).toHaveBeenCalledTimes(3);

    // The responses should be just like regular calls.
    expect(await call1).toEqual(
      BatchCheckPermissionsResponse.fromPartial({
        results: {
          'allowed-get': true,
          set: false,
        },
      }),
    );
    expect(await call2).toEqual(
      BatchCheckPermissionsResponse.fromPartial({
        results: {
          'allowed-get': true,
          delete: false,
        },
      }),
    );
    expect(await call3).toEqual(
      BatchCheckPermissionsResponse.fromPartial({
        results: {
          'allowed-get': true,
          'allowed-set': true,
        },
      }),
    );
    expect(await call4).toEqual(
      BatchCheckPermissionsResponse.fromPartial({
        results: {
          get: false,
          set: false,
        },
      }),
    );
  });
});
