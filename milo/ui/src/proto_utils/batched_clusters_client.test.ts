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
  ClusterRequest,
  ClusterResponse,
  ClustersClientImpl,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';

import { BatchedClustersClientImpl } from './batched_clusters_client';

describe('BatchedClustersClientImpl', () => {
  let clusterSpy: jest.SpiedFunction<BatchedClustersClientImpl['Cluster']>;

  beforeEach(() => {
    jest.useFakeTimers();
    clusterSpy = jest
      .spyOn(ClustersClientImpl.prototype, 'Cluster')
      .mockImplementation(async (req) => {
        return ClusterResponse.fromPartial({
          clusteredTestResults: req.testResults.map((tr) => ({
            requestTag: tr.requestTag,
            clusters: [
              {
                clusterId: { algorithm: 'test-algorithm', id: tr.testId },
              },
            ],
          })),
          clusteringVersion: {
            algorithmsVersion: 1,
            rulesVersion: 'rule-ver',
            configVersion: 'config-ver',
          },
        });
      });
  });

  afterEach(() => {
    jest.useRealTimers();
    clusterSpy.mockReset();
  });

  it('can batch eligible requests together', async () => {
    const client = new BatchedClustersClientImpl(
      {
        request: jest.fn(),
      },
      { maxBatchSize: 5 },
    );

    const call1 = client.Cluster(
      ClusterRequest.fromPartial({
        project: 'project1',
        testResults: [
          { requestTag: 'req1', testId: 'test-1' },
          { requestTag: 'req2', testId: 'test-2' },
        ],
      }),
    );
    const call2 = client.Cluster(
      ClusterRequest.fromPartial({
        project: 'project2',
        testResults: [
          { requestTag: 'req3', testId: 'test-3' },
          { requestTag: 'req4', testId: 'test-4' },
        ],
      }),
    );
    const call3 = client.Cluster(
      ClusterRequest.fromPartial({
        project: 'project1',
        testResults: [
          { requestTag: 'req5', testId: 'test-5' },
          { requestTag: 'req6', testId: 'test-6' },
        ],
      }),
    );

    await jest.advanceTimersToNextTimerAsync();
    // The cluster RPC should be called only once for each `project`.
    expect(clusterSpy).toHaveBeenCalledTimes(2);
    expect(clusterSpy).toHaveBeenCalledWith(
      ClusterRequest.fromPartial({
        project: 'project1',
        testResults: [
          { requestTag: 'req1', testId: 'test-1' },
          { requestTag: 'req2', testId: 'test-2' },
          { requestTag: 'req5', testId: 'test-5' },
          { requestTag: 'req6', testId: 'test-6' },
        ],
      }),
    );
    expect(clusterSpy).toHaveBeenCalledWith(
      ClusterRequest.fromPartial({
        project: 'project2',
        testResults: [
          { requestTag: 'req3', testId: 'test-3' },
          { requestTag: 'req4', testId: 'test-4' },
        ],
      }),
    );
    // The responses should be just like regular calls.
    expect(await call1).toEqual(
      ClusterResponse.fromPartial({
        clusteredTestResults: [
          {
            requestTag: 'req1',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-1' } },
            ],
          },
          {
            requestTag: 'req2',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-2' } },
            ],
          },
        ],
        clusteringVersion: {
          algorithmsVersion: 1,
          rulesVersion: 'rule-ver',
          configVersion: 'config-ver',
        },
      }),
    );
    expect(await call2).toEqual(
      ClusterResponse.fromPartial({
        clusteredTestResults: [
          {
            requestTag: 'req3',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-3' } },
            ],
          },
          {
            requestTag: 'req4',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-4' } },
            ],
          },
        ],
        clusteringVersion: {
          algorithmsVersion: 1,
          rulesVersion: 'rule-ver',
          configVersion: 'config-ver',
        },
      }),
    );
    expect(await call3).toEqual(
      ClusterResponse.fromPartial({
        clusteredTestResults: [
          {
            requestTag: 'req5',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-5' } },
            ],
          },
          {
            requestTag: 'req6',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-6' } },
            ],
          },
        ],
        clusteringVersion: {
          algorithmsVersion: 1,
          rulesVersion: 'rule-ver',
          configVersion: 'config-ver',
        },
      }),
    );
  });

  it('can handle over batching', async () => {
    const client = new BatchedClustersClientImpl(
      {
        request: jest.fn(),
      },
      { maxBatchSize: 5 },
    );

    const call1 = client.Cluster(
      ClusterRequest.fromPartial({
        project: 'project1',
        testResults: [
          { requestTag: 'req1', testId: 'test-1' },
          { requestTag: 'req2', testId: 'test-2' },
        ],
      }),
    );
    const call2 = client.Cluster(
      ClusterRequest.fromPartial({
        project: 'project2',
        testResults: [{ requestTag: 'req3', testId: 'test-3' }],
      }),
    );
    const call3 = client.Cluster(
      ClusterRequest.fromPartial({
        project: 'project1',
        testResults: [
          { requestTag: 'req4', testId: 'test-4' },
          { requestTag: 'req5', testId: 'test-5' },
          { requestTag: 'req6', testId: 'test-6' },
          { requestTag: 'req7', testId: 'test-7' },
        ],
      }),
    );
    const call4 = client.Cluster(
      ClusterRequest.fromPartial({
        project: 'project2',
        testResults: [{ requestTag: 'req8', testId: 'test-8' }],
      }),
    );

    await jest.advanceTimersToNextTimerAsync();
    // The cluster RPC should be called only twice for `project1`, and once for
    // `project2`.
    expect(clusterSpy).toHaveBeenCalledTimes(3);
    expect(clusterSpy).toHaveBeenCalledWith(
      ClusterRequest.fromPartial({
        project: 'project1',
        testResults: [
          { requestTag: 'req1', testId: 'test-1' },
          { requestTag: 'req2', testId: 'test-2' },
        ],
      }),
    );
    expect(clusterSpy).toHaveBeenCalledWith(
      ClusterRequest.fromPartial({
        project: 'project1',
        testResults: [
          { requestTag: 'req4', testId: 'test-4' },
          { requestTag: 'req5', testId: 'test-5' },
          { requestTag: 'req6', testId: 'test-6' },
          { requestTag: 'req7', testId: 'test-7' },
        ],
      }),
    );
    expect(clusterSpy).toHaveBeenCalledWith(
      ClusterRequest.fromPartial({
        project: 'project2',
        testResults: [
          { requestTag: 'req3', testId: 'test-3' },
          { requestTag: 'req8', testId: 'test-8' },
        ],
      }),
    );
    // The responses should be just like regular calls.
    expect(await call1).toEqual(
      ClusterResponse.fromPartial({
        clusteredTestResults: [
          {
            requestTag: 'req1',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-1' } },
            ],
          },
          {
            requestTag: 'req2',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-2' } },
            ],
          },
        ],
        clusteringVersion: {
          algorithmsVersion: 1,
          rulesVersion: 'rule-ver',
          configVersion: 'config-ver',
        },
      }),
    );
    expect(await call2).toEqual(
      ClusterResponse.fromPartial({
        clusteredTestResults: [
          {
            requestTag: 'req3',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-3' } },
            ],
          },
        ],
        clusteringVersion: {
          algorithmsVersion: 1,
          rulesVersion: 'rule-ver',
          configVersion: 'config-ver',
        },
      }),
    );
    expect(await call3).toEqual(
      ClusterResponse.fromPartial({
        clusteredTestResults: [
          {
            requestTag: 'req4',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-4' } },
            ],
          },
          {
            requestTag: 'req5',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-5' } },
            ],
          },
          {
            requestTag: 'req6',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-6' } },
            ],
          },
          {
            requestTag: 'req7',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-7' } },
            ],
          },
        ],
        clusteringVersion: {
          algorithmsVersion: 1,
          rulesVersion: 'rule-ver',
          configVersion: 'config-ver',
        },
      }),
    );
    expect(await call4).toEqual(
      ClusterResponse.fromPartial({
        clusteredTestResults: [
          {
            requestTag: 'req8',
            clusters: [
              { clusterId: { algorithm: 'test-algorithm', id: 'test-8' } },
            ],
          },
        ],
        clusteringVersion: {
          algorithmsVersion: 1,
          rulesVersion: 'rule-ver',
          configVersion: 'config-ver',
        },
      }),
    );
  });
});
