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

import {
  BatchGetTestVariantsRequest,
  BatchGetTestVariantsResponse,
  ResultDBClientImpl,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';

import { BatchedResultDBClientImpl } from './resultdb_client';

describe('BatchedResultDBClientImpl', () => {
  let batchGetTestVariants: jest.SpiedFunction<
    BatchedResultDBClientImpl['BatchGetTestVariants']
  >;

  beforeEach(() => {
    jest.useFakeTimers();
    batchGetTestVariants = jest
      .spyOn(ResultDBClientImpl.prototype, 'BatchGetTestVariants')
      .mockImplementation(async (batchReq) => {
        return BatchGetTestVariantsResponse.fromPartial({
          testVariants: batchReq.testVariants.map((tv) => ({
            testId: tv.testId,
            variantHash: tv.variantHash,
            sourcesId: tv.variantHash.split('-')[0],
          })),
          sources: Object.fromEntries(
            batchReq.testVariants.map((tv) => [
              tv.variantHash.split('-')[0],
              {
                gitilesCommit: { commitHash: tv.variantHash.split('-')[0] },
              },
            ]),
          ),
        });
      });
  });

  afterEach(() => {
    jest.useRealTimers();
    batchGetTestVariants.mockReset();
  });

  it('can batch eligible requests together', async () => {
    const client = new BatchedResultDBClientImpl(
      {
        request: jest.fn(),
      },
      { maxBatchSize: 5 },
    );

    const call1 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv1',
        resultLimit: 10,
        testVariants: [
          { testId: 'test1', variantHash: 'source1-hash1' },
          { testId: 'test2', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call2 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv2',
        resultLimit: 10,
        testVariants: [
          { testId: 'test3', variantHash: 'source1-hash1' },
          { testId: 'test4', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call3 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv1',
        resultLimit: 10,
        testVariants: [
          { testId: 'test5', variantHash: 'source1-hash1' },
          { testId: 'test6', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call4 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv2',
        resultLimit: 5,
        testVariants: [
          { testId: 'test7', variantHash: 'source1-hash1' },
          { testId: 'test8', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call5 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv2',
        resultLimit: 10,
        testVariants: [{ testId: 'test9', variantHash: 'source1-hash1' }],
      }),
    );

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called only once for each
    // `invocation` + `resultLimit` combination.
    expect(batchGetTestVariants).toHaveBeenCalledTimes(3);
    expect(batchGetTestVariants).toHaveBeenCalledWith(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv1',
        resultLimit: 10,
        testVariants: [
          { testId: 'test1', variantHash: 'source1-hash1' },
          { testId: 'test2', variantHash: 'source2-hash1' },
          { testId: 'test5', variantHash: 'source1-hash1' },
          { testId: 'test6', variantHash: 'source2-hash1' },
        ],
      }),
    );
    expect(batchGetTestVariants).toHaveBeenCalledWith(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv2',
        resultLimit: 10,
        testVariants: [
          { testId: 'test3', variantHash: 'source1-hash1' },
          { testId: 'test4', variantHash: 'source2-hash1' },
          { testId: 'test9', variantHash: 'source1-hash1' },
        ],
      }),
    );
    expect(batchGetTestVariants).toHaveBeenCalledWith(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv2',
        resultLimit: 5,
        testVariants: [
          { testId: 'test7', variantHash: 'source1-hash1' },
          { testId: 'test8', variantHash: 'source2-hash1' },
        ],
      }),
    );

    // The responses should be just like regular calls.
    expect(await call1).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test1',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test2',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call2).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test3',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test4',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call3).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test5',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test6',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call4).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test7',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test8',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call5).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test9',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
        },
      }),
    );
  });

  it('can handle over batching', async () => {
    const client = new BatchedResultDBClientImpl(
      {
        request: jest.fn(),
      },
      { maxBatchSize: 5 },
    );

    const call1 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv1',
        resultLimit: 10,
        testVariants: [
          { testId: 'test1', variantHash: 'source1-hash1' },
          { testId: 'test2', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call2 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv2',
        resultLimit: 10,
        testVariants: [
          { testId: 'test3', variantHash: 'source1-hash1' },
          { testId: 'test4', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call3 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv1',
        resultLimit: 10,
        testVariants: [
          { testId: 'test5', variantHash: 'source1-hash1' },
          { testId: 'test6', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call4 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv2',
        resultLimit: 10,
        testVariants: [
          { testId: 'test7', variantHash: 'source1-hash1' },
          { testId: 'test8', variantHash: 'source2-hash1' },
        ],
      }),
    );
    const call5 = client.BatchGetTestVariants(
      BatchGetTestVariantsRequest.fromPartial({
        invocation: 'invocations/inv1',
        resultLimit: 10,
        testVariants: [
          { testId: 'test9', variantHash: 'source1-hash1' },
          { testId: 'test10', variantHash: 'source1-hash2' },
        ],
      }),
    );

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called 3 times.
    // 2 time2 for inv1, 1 time for inv2.
    expect(batchGetTestVariants).toHaveBeenCalledTimes(3);

    // The responses should be just like regular calls.
    expect(await call1).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test1',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test2',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call2).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test3',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test4',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call3).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test5',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test6',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call4).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test7',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test8',
            variantHash: 'source2-hash1',
            sourcesId: 'source2',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
          source2: {
            gitilesCommit: {
              commitHash: 'source2',
            },
          },
        },
      }),
    );
    expect(await call5).toEqual(
      BatchGetTestVariantsResponse.fromPartial({
        testVariants: [
          {
            testId: 'test9',
            variantHash: 'source1-hash1',
            sourcesId: 'source1',
          },
          {
            testId: 'test10',
            variantHash: 'source1-hash2',
            sourcesId: 'source1',
          },
        ],
        sources: {
          source1: {
            gitilesCommit: {
              commitHash: 'source1',
            },
          },
        },
      }),
    );
  });
});
