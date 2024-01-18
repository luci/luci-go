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

import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import {
  BatchRequest,
  BatchRequest_Request,
  BatchResponse,
  BatchResponse_Response,
  BuildsClientImpl,
  GetBuildRequest,
  GetBuildStatusRequest,
  SearchBuildsRequest,
  SearchBuildsResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

import { BatchedBuildsClientImpl } from './batched_builds_client';

const getBuildReq = GetBuildRequest.fromPartial({
  id: '1234',
});
const getBuildRes = Build.fromPartial({
  id: '1234',
});

const searchBuildsReq = SearchBuildsRequest.fromPartial({
  predicate: {
    builder: {
      project: 'proj',
      bucket: 'bucket',
      builder: 'builder1',
    },
  },
});
const searchBuildsRes = SearchBuildsResponse.fromPartial({
  builds: Object.freeze([
    {
      id: '2345',
    },
  ]),
});

const getBuildStatusReq = GetBuildStatusRequest.fromPartial({
  id: '3456',
});
const getBuildStatusRes = Build.fromPartial({
  id: '3456',
});

const batchReq = BatchRequest.fromPartial({
  requests: Object.freeze([
    BatchRequest_Request.fromPartial({
      getBuild: getBuildReq,
    }),
    BatchRequest_Request.fromPartial({
      searchBuilds: searchBuildsReq,
    }),
    BatchRequest_Request.fromPartial({
      getBuildStatus: getBuildStatusReq,
    }),
  ]),
});
const batchRes = BatchResponse.fromPartial({
  responses: Object.freeze([
    { getBuild: getBuildRes },
    { searchBuilds: searchBuildsRes },
    { getBuildStatus: getBuildStatusRes },
  ]),
});

describe('BatchedBuildsClientImpl', () => {
  let batchSpy: jest.SpiedFunction<BuildsClientImpl['Batch']>;
  let getBuildSpy: jest.SpiedFunction<BuildsClientImpl['GetBuild']>;
  let searchBuildsSpy: jest.SpiedFunction<BuildsClientImpl['SearchBuilds']>;
  let getBuildStatusSpy: jest.SpiedFunction<BuildsClientImpl['GetBuildStatus']>;

  beforeEach(() => {
    jest.useFakeTimers();
    batchSpy = jest
      .spyOn(BuildsClientImpl.prototype, 'Batch')
      .mockImplementation(async (batchReq) => {
        return BatchResponse.fromPartial({
          responses: Object.freeze(
            batchReq.requests.map((req) => {
              if (req.getBuild) {
                return BatchResponse_Response.fromPartial({
                  getBuild: getBuildRes,
                });
              }
              if (req.searchBuilds) {
                return BatchResponse_Response.fromPartial({
                  searchBuilds: searchBuildsRes,
                });
              }
              if (req.getBuildStatus) {
                return BatchResponse_Response.fromPartial({
                  getBuildStatus: getBuildStatusRes,
                });
              }
              throw Error('unimplemented');
            }),
          ),
        });
      });
    getBuildSpy = jest.spyOn(BuildsClientImpl.prototype, 'GetBuild');
    searchBuildsSpy = jest.spyOn(BuildsClientImpl.prototype, 'SearchBuilds');
    getBuildStatusSpy = jest.spyOn(
      BuildsClientImpl.prototype,
      'GetBuildStatus',
    );
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('can batch eligible requests together', async () => {
    const client = new BatchedBuildsClientImpl({ request: jest.fn() });

    const getBuildCall = client.GetBuild(getBuildReq);
    const searchBuildsCall = client.SearchBuilds(searchBuildsReq);
    const getBuildStatusCall = client.GetBuildStatus(getBuildStatusReq);
    const batchCall = client.Batch(batchReq);

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called only once.
    expect(batchSpy).toHaveBeenCalledTimes(1);
    expect(batchSpy).toHaveBeenCalledWith(
      BatchRequest.fromPartial({
        requests: Object.freeze([
          {
            getBuild: getBuildReq,
          },
          {
            searchBuilds: searchBuildsReq,
          },
          {
            getBuildStatus: getBuildStatusReq,
          },
          // The following are merged from a "Batch" call.
          {
            getBuild: getBuildReq,
          },
          {
            searchBuilds: searchBuildsReq,
          },
          {
            getBuildStatus: getBuildStatusReq,
          },
        ]),
      }),
    );

    // The responses should be just like regular calls.
    expect(await getBuildCall).toEqual(getBuildRes);
    expect(await searchBuildsCall).toEqual(searchBuildsRes);
    expect(await getBuildStatusCall).toEqual(getBuildStatusRes);
    expect(await batchCall).toEqual(batchRes);

    // Original RPCs not called.
    expect(getBuildSpy).not.toHaveBeenCalled();
    expect(searchBuildsSpy).not.toHaveBeenCalled();
    expect(getBuildStatusSpy).not.toHaveBeenCalled();
  });

  it('can handle over batching', async () => {
    const client = new BatchedBuildsClientImpl({ request: jest.fn() });

    const getBuildCalls = Array(30)
      .fill(0)
      .map(() => client.GetBuild(getBuildReq));
    const batchCalls = Array(30)
      .fill(0)
      .map(() => client.Batch(batchReq));
    const getBuildStatusCalls = Array(30)
      .fill(0)
      .map(() => client.GetBuildStatus(getBuildStatusReq));
    const otherBatchCalls = Array(30)
      .fill(0)
      .map(() => client.Batch(batchReq));

    await jest.advanceTimersToNextTimerAsync();
    // The batch function should be called more than once.
    expect(batchSpy).toHaveBeenCalledTimes(3);

    // The responses should be just like regular calls.
    expect(await Promise.all(getBuildCalls)).toEqual(
      Array(30).fill(getBuildRes),
    );
    expect(await Promise.all(batchCalls)).toEqual(Array(30).fill(batchRes));
    expect(await Promise.all(getBuildStatusCalls)).toEqual(
      Array(30).fill(getBuildStatusRes),
    );
    expect(await Promise.all(otherBatchCalls)).toEqual(
      Array(30).fill(batchRes),
    );
  });
});
