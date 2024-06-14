// Copyright 2022 The LUCI Authors.
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

import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';
import { DeepMutable } from '@/generic_libs/types';

import {
  ClusterRequest,
  ClusterResponse,
  ClustersService,
} from './luci_analysis';

const clusteringVersion = {
  algorithmsVersion: '1',
  rulesVersion: '1',
  configVersion: '1',
};

describe('ClustersService', () => {
  test('should batch requests from the same project together', async () => {
    const prpc = new PrpcClientExt({}, () => '');
    const callStub = jest.spyOn(prpc, 'call');
    const clustersService = new ClustersService(prpc);
    const mockedRes: ClusterResponse = {
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '1' } }] },
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '2' } }] },
      ],
    };
    callStub.mockResolvedValueOnce(mockedRes);

    const call1 = clustersService.cluster({
      project: 'proj1',
      testResults: [{ testId: 'test1' }],
    });
    const call2 = clustersService.cluster({
      project: 'proj1',
      testResults: [{ testId: 'test2' }],
    });
    const res1 = await call1;
    const res2 = await call2;

    expect(callStub.mock.calls.length).toStrictEqual(1);
    expect(callStub.mock.calls[0]).toEqual([
      'luci.analysis.v1.Clusters',
      'Cluster',
      {
        project: 'proj1',
        testResults: [{ testId: 'test1' }, { testId: 'test2' }],
      },
    ]);
    expect(res1).toEqual({
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '1' } }] },
      ],
    });
    expect(res2).toEqual({
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '2' } }] },
      ],
    });
  });

  test('should not batch requests from different projects together', async () => {
    const prpc = new PrpcClientExt({}, () => '');
    const callStub = jest.spyOn(prpc, 'call');
    const clustersService = new ClustersService(prpc);
    const mockedRes1: ClusterResponse = {
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '1' } }] },
      ],
    };
    const mockedRes2: ClusterResponse = {
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '2' } }] },
      ],
    };
    callStub.mockResolvedValueOnce(mockedRes1);
    callStub.mockResolvedValueOnce(mockedRes2);

    const call1 = clustersService.cluster({
      project: 'proj1',
      testResults: [{ testId: 'test1' }],
    });
    const call2 = clustersService.cluster({
      project: 'proj2',
      testResults: [{ testId: 'test2' }],
    });
    const res1 = await call1;
    const res2 = await call2;

    expect(callStub.mock.calls.length).toStrictEqual(2);
    expect(callStub.mock.calls[0]).toEqual([
      'luci.analysis.v1.Clusters',
      'Cluster',
      {
        project: 'proj1',
        testResults: [{ testId: 'test1' }],
      },
    ]);
    expect(callStub.mock.calls[1]).toEqual([
      'luci.analysis.v1.Clusters',
      'Cluster',
      {
        project: 'proj2',
        testResults: [{ testId: 'test2' }],
      },
    ]);
    expect(res1).toEqual({
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '1' } }] },
      ],
    });
    expect(res2).toEqual({
      clusteringVersion,
      clusteredTestResults: [
        { clusters: [{ clusterId: { algorithm: 'algorithm', id: '2' } }] },
      ],
    });
  });

  test('should not batch more than 1000 requests together', async () => {
    const prpc = new PrpcClientExt({}, () => '');
    const callStub = jest.spyOn(prpc, 'call');
    const clustersService = new ClustersService(prpc);

    function makeRes(startIndex: number, count: number) {
      const res: DeepMutable<ClusterResponse> = {
        clusteringVersion,
        clusteredTestResults: [],
      };
      for (let i = startIndex; i < count + startIndex; ++i) {
        res.clusteredTestResults.push({
          clusters: [{ clusterId: { algorithm: 'algorithm', id: `${i}` } }],
        });
      }
      return res;
    }

    function makeReq(startIndex: number, count: number) {
      const req: DeepMutable<ClusterRequest> = {
        project: 'proj1',
        testResults: [],
      };
      for (let i = startIndex; i < count + startIndex; ++i) {
        req.testResults.push({ testId: `test${i}` });
      }
      return req;
    }

    const mockedRes1 = makeRes(0, 800);
    const mockedRes2 = makeRes(800, 400);
    callStub.mockResolvedValueOnce(mockedRes1);
    callStub.mockResolvedValueOnce(mockedRes2);

    const call1 = clustersService.cluster(makeReq(0, 400));
    const call2 = clustersService.cluster(makeReq(400, 400));
    const call3 = clustersService.cluster(makeReq(800, 400));
    const res1 = await call1;
    const res2 = await call2;
    const res3 = await call3;

    expect(callStub.mock.calls.length).toStrictEqual(2);
    expect(callStub.mock.calls[0]).toEqual([
      'luci.analysis.v1.Clusters',
      'Cluster',
      makeReq(0, 800),
    ]);
    expect(callStub.mock.calls[1]).toEqual([
      'luci.analysis.v1.Clusters',
      'Cluster',
      makeReq(800, 400),
    ]);
    expect(res1).toEqual(makeRes(0, 400));
    expect(res2).toEqual(makeRes(400, 400));
    expect(res3).toEqual(makeRes(800, 400));
  });
});
