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

import { Analysis, ListAnalysesResponse } from '../../services/luci_bisection';

export const createMockListAnalysesResponse = (
  analyses: Analysis[],
  nextPageToken: string
): ListAnalysesResponse => {
  return {
    analyses: analyses,
    nextPageToken: nextPageToken,
  };
};

export const mockFetchAnalyses = (
  mockAnalyses: Analysis[],
  nextPageToken: string
) => {
  fetchMock.post(
    'http://localhost/prpc/luci.bisection.v1.Analyses/ListAnalyses',
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
      body:
        ")]}'" +
        JSON.stringify(
          createMockListAnalysesResponse(mockAnalyses, nextPageToken)
        ),
    },
    { overwriteRoutes: true }
  );
};

export const mockErrorFetchingAnalyses = () => {
  fetchMock.post(
    'http://localhost/prpc/luci.bisection.v1.Analyses/ListAnalyses',
    {
      headers: {
        'X-Prpc-Grpc-Code': '2',
      },
    }
  );
};
