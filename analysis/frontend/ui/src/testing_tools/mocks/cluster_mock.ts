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

import fetchMock from 'fetch-mock-jest';

import {
  Cluster,
  ClusterExoneratedTestVariant,
  ClusterSummary,
  DistinctClusterFailure,
  GetClusterRequest,
  QueryClusterExoneratedTestVariantsRequest,
  QueryClusterExoneratedTestVariantsResponse,
  QueryClusterFailuresRequest,
  QueryClusterFailuresResponse,
  QueryClusterHistoryResponse,
  QueryClusterSummariesRequest,
  QueryClusterSummariesResponse,
  ReclusteringProgress,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/clusters.pb';
import { DeepMutable } from '@/types/types';

export const getMockCluster = (id: string,
    project = 'testproject',
    algorithm = 'reason-v2',
    title = ''): DeepMutable<Cluster> => {
  return {
    name: `projects/${project}/clusters/${algorithm}/${id}`,
    hasExample: true,
    title: title,
    metrics: {
      'human-cls-failed-presubmit': {
        oneDay: { nominal: '98' },
        threeDay: { nominal: '158' },
        sevenDay: { nominal: '167' },
      },
      'critical-failures-exonerated': {
        oneDay: { nominal: '5625' },
        threeDay: { nominal: '14052' },
        sevenDay: { nominal: '13800' },
      },
      'failures': {
        oneDay: { nominal: '7625' },
        threeDay: { nominal: '16052' },
        sevenDay: { nominal: '15800' },
      },
    },
    equivalentFailureAssociationRule: '',
  };
};

export const getMockRuleBasicClusterSummary = (id: string): DeepMutable<ClusterSummary> => {
  return {
    clusterId: {
      'algorithm': 'rules-v2',
      'id': id,
    },
    title: 'reason LIKE "blah%"',
    bug: {
      'system': 'buganizer',
      'id': '123456789',
      'linkText': 'b/123456789',
      'url': 'https://buganizer/123456789',
    },
    metrics: {
      'human-cls-failed-presubmit': {
        value: '27',
        dailyBreakdown: [],
      },
      'critical-failures-exonerated': {
        value: '918',
        dailyBreakdown: [],
      },
      'failures': {
        value: '1871',
        dailyBreakdown: [],
      },
    },
  };
};

export const getMockRuleFullClusterSummary = (id: string): DeepMutable<ClusterSummary> => {
  return {
    clusterId: {
      'algorithm': 'rules-v2',
      'id': id,
    },
    title: 'reason LIKE "blah%"',
    bug: {
      'system': 'buganizer',
      'id': '123456789',
      'linkText': 'b/123456789',
      'url': 'https://buganizer/123456789',
    },
    metrics: {
      'human-cls-failed-presubmit': {
        value: '27',
        dailyBreakdown: new Array(7).fill('1'),
      },
      'critical-failures-exonerated': {
        value: '918',
        dailyBreakdown: new Array(7).fill('2'),
      },
      'failures': {
        value: '1871',
        dailyBreakdown: new Array(7).fill('3'),
      },
    },
  };
};

export const getMockSuggestedBasicClusterSummary = (id: string, algorithm = 'reason-v3'): ClusterSummary => {
  return {
    clusterId: {
      'algorithm': algorithm,
      'id': id,
    },
    bug: undefined,
    title: 'reason LIKE "blah%"',
    metrics: {
      'human-cls-failed-presubmit': {
        value: '29',
        dailyBreakdown: [],
      },
      'critical-failures-exonerated': {
        value: '919',
        dailyBreakdown: [],
      },
      'failures': {
        value: '1872',
        dailyBreakdown: [],
      },
    },
  };
};

export const getMockSuggestedFullClusterSummary = (id: string, algorithm = 'reason-v3'): ClusterSummary => {
  return {
    clusterId: {
      'algorithm': algorithm,
      'id': id,
    },
    bug: undefined,
    title: 'reason LIKE "blah%"',
    metrics: {
      'human-cls-failed-presubmit': {
        value: '29',
        dailyBreakdown: new Array(7).fill('4'),
      },
      'critical-failures-exonerated': {
        value: '919',
        dailyBreakdown: new Array(7).fill('5'),
      },
      'failures': {
        value: '1872',
        dailyBreakdown: new Array(7).fill('6'),
      },
    },
  };
};

export const getMockClusterExoneratedTestVariant = (id: string, exoneratedFailures: number): ClusterExoneratedTestVariant => {
  return {
    testId: id,
    variant: undefined,
    criticalFailuresExonerated: exoneratedFailures,
    lastExoneration: '2052-01-02T03:04:05.678901234Z',
  };
};

export const mockQueryClusterSummaries = (request: QueryClusterSummariesRequest, response: QueryClusterSummariesResponse, overwriteRoutes = true) => {
  fetchMock.post({
    url: 'http://localhost/prpc/luci.analysis.v1.Clusters/QueryClusterSummaries',
    body: QueryClusterSummariesRequest.toJSON(request) as object,
  }, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n' + JSON.stringify(QueryClusterSummariesResponse.toJSON(response)),
  }, { overwriteRoutes: overwriteRoutes });
};

export const mockGetCluster = (
    project: string,
    algorithm: string,
    id: string,
    response: Cluster) => {
  const request: GetClusterRequest = {
    name: `projects/${encodeURIComponent(project)}/clusters/${encodeURIComponent(algorithm)}/${encodeURIComponent(id)}`,
  };

  fetchMock.post({
    url: 'http://localhost/prpc/luci.analysis.v1.Clusters/Get',
    body: request,
  }, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n' + JSON.stringify(Cluster.toJSON(response) as object),
  }, { overwriteRoutes: true });
};

export const mockQueryClusterFailures = (request: QueryClusterFailuresRequest, failures: DistinctClusterFailure[]) => {
  const response: QueryClusterFailuresResponse = {
    failures: failures,
  };
  fetchMock.post({
    url: 'http://localhost/prpc/luci.analysis.v1.Clusters/QueryClusterFailures',
    body: QueryClusterFailuresRequest.toJSON(request) as object,
  }, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n' + JSON.stringify(QueryClusterFailuresResponse.toJSON(response) as object),
  }, { overwriteRoutes: true });
};

export const mockQueryExoneratedTestVariants = (parent: string, testVariants: ClusterExoneratedTestVariant[]) => {
  const request: QueryClusterExoneratedTestVariantsRequest = {
    parent: parent,
  };
  const response: QueryClusterExoneratedTestVariantsResponse = {
    testVariants: testVariants,
  };
  fetchMock.post({
    url: 'http://localhost/prpc/luci.analysis.v1.Clusters/QueryExoneratedTestVariants',
    body: QueryClusterExoneratedTestVariantsRequest.toJSON(request) as object,
  }, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n' + JSON.stringify(QueryClusterExoneratedTestVariantsResponse.toJSON(response)),
  }, { overwriteRoutes: true });
};

export const mockQueryHistory = (
    response: QueryClusterHistoryResponse) => {
  fetchMock.post({
    url: 'http://localhost/prpc/luci.analysis.v1.Clusters/QueryHistory',
  }, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n' + JSON.stringify(QueryClusterHistoryResponse.toJSON(response)),
  }, { overwriteRoutes: true });
};

export const mockReclusteringProgress = (response: ReclusteringProgress) => {
  fetchMock.post('http://localhost/prpc/luci.analysis.v1.Clusters/GetReclusteringProgress', {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n'+JSON.stringify(ReclusteringProgress.toJSON(response)),
  }, { overwriteRoutes: true });
};
