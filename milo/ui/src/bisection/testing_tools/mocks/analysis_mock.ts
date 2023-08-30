// Copyright 2023 The LUCI Authors.
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
  Analysis,
  QueryAnalysisResponse,
} from '@/common/services/luci_bisection';

export const createMockAnalysis = (id: string): Analysis => {
  return {
    analysisId: id,
    status: 'FOUND',
    runStatus: 'ENDED',
    lastPassedBbid: '0',
    firstFailedBbid: id,
    createdTime: '2022-09-06T07:13:16.398865Z',
    lastUpdatedTime: '2022-09-06T07:13:16.893998Z',
    endTime: '2022-09-06T07:13:16.893998Z',
    builder: {
      project: 'chromium/test',
      bucket: 'ci',
      builder: 'mock-builder-cc64',
    },
    buildFailureType: 'COMPILE',
    heuristicResult: {
      status: 'NOTFOUND',
    },
    nthSectionResult: {
      status: 'RUNNING',
      remainingNthSectionRange: {
        lastPassed: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'abc123abc123',
          position: '102',
        },
        firstFailed: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'def456def456',
          position: '103',
        },
      },
      startTime: '2022-09-06T07:13:16.398865Z',
      endTime: '2022-09-06T07:13:16.398865Z',
      reruns: [],
      blameList: {
        commits: [],
      },
    },
  };
};

const createMockQueryAnalysisResponse = (
  analyses: Analysis[],
): QueryAnalysisResponse => {
  return {
    analyses: analyses,
  };
};

export const mockQueryAnalysis = (mockAnalyses: Analysis[]) => {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/QueryAnalysis',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body:
        ")]}'" + JSON.stringify(createMockQueryAnalysisResponse(mockAnalyses)),
    },
    { overwriteRoutes: true },
  );
};

export const mockErrorQueryingAnalysis = () => {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/QueryAnalysis',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
  );
};
