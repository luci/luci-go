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
  BatchGetTestVariantBranchRequest,
  BatchGetTestVariantBranchResponse,
  TestVariantBranch,
  TestVariantBranchesClientImpl,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variant_branches.pb';

import { BatchedTestVariantBranchesClientImpl } from './test_variant_branches_client';

describe('BatchedTestVariantBranchesClientImpl', () => {
  let batchGetSpy: jest.SpiedFunction<
    BatchedTestVariantBranchesClientImpl['BatchGet']
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    batchGetSpy = jest
      .spyOn(TestVariantBranchesClientImpl.prototype, 'BatchGet')
      .mockImplementation(async (batchReq) => {
        return BatchGetTestVariantBranchResponse.fromPartial({
          testVariantBranches: batchReq.names.map((name) =>
            TestVariantBranch.fromPartial({
              name,
            }),
          ),
        });
      });
  });

  afterEach(() => {
    jest.useRealTimers();
    batchGetSpy.mockReset();
  });

  it('can batch eligible requests together', async () => {
    const client = new BatchedTestVariantBranchesClientImpl({
      request: jest.fn(),
    });
    const CALL_COUNT = 3;
    const BATCH_SIZE = 30;

    const calls = Array(CALL_COUNT)
      .fill(0)
      .map((_, i) =>
        client.BatchGet(
          BatchGetTestVariantBranchRequest.fromPartial({
            names: Array(BATCH_SIZE)
              .fill(0)
              .map((_, j) => `call${i}-name${j}`),
          }),
        ),
      );

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called only once.
    expect(batchGetSpy).toHaveBeenCalledTimes(1);
    expect(batchGetSpy).toHaveBeenCalledWith(
      BatchGetTestVariantBranchRequest.fromPartial({
        names: Array(CALL_COUNT)
          .fill(0)
          .flatMap((_, i) =>
            Array(BATCH_SIZE)
              .fill(0)
              .map((_, j) => `call${i}-name${j}`),
          ),
      }),
    );

    // The responses should be just like regular calls.
    expect(await Promise.all(calls)).toEqual(
      Array(CALL_COUNT)
        .fill(0)
        .map((_, i) =>
          BatchGetTestVariantBranchResponse.fromPartial({
            testVariantBranches: Array(BATCH_SIZE)
              .fill(0)
              .map((_, j) => ({ name: `call${i}-name${j}` })),
          }),
        ),
    );
  });

  it('can handle over batching', async () => {
    const client = new BatchedTestVariantBranchesClientImpl({
      request: jest.fn(),
    });
    const CALL_COUNT = 5;
    const BATCH_SIZE = 30;

    const calls = Array(CALL_COUNT)
      .fill(0)
      .map((_, i) =>
        client.BatchGet(
          BatchGetTestVariantBranchRequest.fromPartial({
            names: Array(BATCH_SIZE)
              .fill(0)
              .map((_, j) => `call${i}-name${j}`),
          }),
        ),
      );

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called more than once.
    expect(batchGetSpy).toHaveBeenCalledTimes(2);

    // The responses should be just like regular calls.
    expect(await Promise.all(calls)).toEqual(
      Array(CALL_COUNT)
        .fill(0)
        .map((_, i) =>
          BatchGetTestVariantBranchResponse.fromPartial({
            testVariantBranches: Array(BATCH_SIZE)
              .fill(0)
              .map((_, j) => ({ name: `call${i}-name${j}` })),
          }),
        ),
    );
  });
});
