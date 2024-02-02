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
  AnalysisRunStatus,
  ListTestAnalysesResponse,
  TestAnalysis,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';

export function createMockTestAnalysis(id: string) {
  return TestAnalysis.fromPartial({
    analysisId: id,
    status: AnalysisStatus.FOUND,
    runStatus: AnalysisRunStatus.ENDED,
    createdTime: '2022-09-06T07:13:16.398865Z',
    endTime: '2022-09-06T07:13:16.893998Z',
    builder: {
      project: 'chromium/test',
      bucket: 'ci',
      builder: 'mock-builder-cc64',
    },
    testFailures: Object.freeze([
      {
        testId: 'test1',
        variant: {
          def: { os: 'linux' },
        },
        variantHash: 'variant1',
        isDiverged: false,
        isPrimary: true,
        startHour: '2022-09-06T03:00:00Z',
      },
    ]),
    sampleBbid: '1234',
    nthSectionResult: {
      status: AnalysisStatus.RUNNING,
      remainingNthSectionRange: {
        lastPassed: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'abc123abc123',
          position: 102,
        },
        firstFailed: {
          host: 'testHost',
          project: 'testProject',
          ref: 'test/ref/dev',
          id: 'def456def456',
          position: 103,
        },
      },
      startTime: '2022-09-06T07:13:16.398865Z',
      endTime: '2022-09-06T07:13:16.398865Z',
      blameList: {},
    },
  });
}

function createMockListTestAnalysesResponse(
  analyses: readonly TestAnalysis[],
  nextPageToken: string,
) {
  return ListTestAnalysesResponse.fromPartial({
    analyses,
    nextPageToken,
  });
}

export function mockListTestAnalyses(
  mockAnalyses: readonly TestAnalysis[],
  nextPageToken: string,
) {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/ListTestAnalyses',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body:
        ")]}'\n" +
        JSON.stringify(
          ListTestAnalysesResponse.toJSON(
            createMockListTestAnalysesResponse(mockAnalyses, nextPageToken),
          ),
        ),
    },
    { overwriteRoutes: true },
  );
}

export function mockErrorListTestAnalyses() {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/ListTestAnalyses',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
  );
}

export function mockGetTestAnalysis(mockAnalysis: TestAnalysis) {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/GetTestAnalysis',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body: ")]}'\n" + JSON.stringify(TestAnalysis.toJSON(mockAnalysis)),
    },
    { overwriteRoutes: true },
  );
}

export function mockErrorGetTestAnalysis(rpcCode: number) {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/GetTestAnalysis',
    {
      headers: {
        'X-Prpc-Grpc-Code': rpcCode,
      },
    },
  );
}
