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
  AnalysisRunStatus,
  BuildFailureType,
  QueryAnalysisResponse,
} from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { AnalysisStatus } from '@/proto/go.chromium.org/luci/bisection/proto/v1/common.pb';
import { Build } from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';

export function createMockAnalysis(id: string) {
  return Analysis.fromPartial({
    analysisId: id,
    status: AnalysisStatus.FOUND,
    runStatus: AnalysisRunStatus.ENDED,
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
    buildFailureType: BuildFailureType.COMPILE,
    genAiResult: {
      status: AnalysisStatus.NOTFOUND,
    },
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

function createMockQueryAnalysisResponse(analyses: readonly Analysis[]) {
  return QueryAnalysisResponse.fromPartial({
    analyses: analyses,
  });
}

export function mockQueryAnalysis(mockAnalyses: readonly Analysis[]) {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/QueryAnalysis',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body:
        ")]}'\n" +
        JSON.stringify(
          QueryAnalysisResponse.toJSON(
            createMockQueryAnalysisResponse(mockAnalyses),
          ),
        ),
    },
    { overwriteRoutes: true },
  );
}

export function mockErrorQueryingAnalysis() {
  fetchMock.post(
    'https://' +
      SETTINGS.luciBisection.host +
      '/prpc/luci.bisection.v1.Analyses/QueryAnalysis',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
    { overwriteRoutes: true },
  );
}

export function mockGetBuild(bbid: string, build: Build) {
  fetchMock.post(
    'https://' +
      SETTINGS.buildbucket.host +
      '/prpc/buildbucket.v2.Builds/GetBuild',
    (_, opts) => {
      if (opts.body && JSON.parse(opts.body as string).id === bbid) {
        return {
          headers: {
            'X-Prpc-Grpc-Code': '0',
          },
          body: ")]}'\n" + JSON.stringify(Build.toJSON(build)),
        };
      }
      return {
        headers: {
          'X-Prpc-Grpc-Code': '5' /* NOT_FOUND */,
        },
      };
    },
    { overwriteRoutes: true },
  );
}

export function mockErrorQueryingBuild() {
  fetchMock.post(
    'https://' +
      SETTINGS.buildbucket.host +
      '/prpc/buildbucket.v2.Builds/GetBuild',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    },
    { overwriteRoutes: true },
  );
}

export function mockNoBuild() {
  fetchMock.post(
    'https://' +
      SETTINGS.buildbucket.host +
      '/prpc/buildbucket.v2.Builds/GetBuild',
    {
      headers: {
        'X-Prpc-Grpc-Code': '5' /* NOT_FOUND */,
      },
    },
    { overwriteRoutes: true },
  );
}
