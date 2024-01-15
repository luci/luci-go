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
  TestVariantIdentifier,
  TestVariantFailureRateAnalysis,
  QueryTestVariantFailureRateRequest,
  QueryTestVariantFailureRateResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';

export const getMockTestVariantIdentifier = (id: string): TestVariantIdentifier => {
  return TestVariantIdentifier.create({
    'testId': id,
  });
};

export const getMockTestVariantFailureRateAnalysis = (id: string): TestVariantFailureRateAnalysis => {
  return TestVariantFailureRateAnalysis.create({
    testId: id,
    intervalStats: Object.freeze([{
      intervalAge: 1,
      totalRunExpectedVerdicts: 101,
      totalRunFlakyVerdicts: 102,
      totalRunUnexpectedVerdicts: 103,
    }, {
      intervalAge: 2,
      totalRunExpectedVerdicts: 0,
      totalRunFlakyVerdicts: 0,
      totalRunUnexpectedVerdicts: 0,
    }, {
      intervalAge: 3,
      totalRunExpectedVerdicts: 0,
      totalRunFlakyVerdicts: 0,
      totalRunUnexpectedVerdicts: 0,
    }, {
      intervalAge: 4,
      totalRunExpectedVerdicts: 0,
      totalRunFlakyVerdicts: 0,
      totalRunUnexpectedVerdicts: 0,
    }, {
      intervalAge: 5,
      totalRunExpectedVerdicts: 0,
      totalRunFlakyVerdicts: 0,
      totalRunUnexpectedVerdicts: 0,
    }]),
  });
};

export const mockQueryFailureRate = (request: QueryTestVariantFailureRateRequest, response: QueryTestVariantFailureRateResponse) => {
  fetchMock.post({
    url: 'http://localhost/prpc/luci.analysis.v1.TestVariants/QueryFailureRate',
    body: QueryTestVariantFailureRateRequest.toJSON(request) as object,
  }, {
    headers: {
      'X-Prpc-Grpc-Code': '0',
    },
    body: ')]}\'\n' + JSON.stringify(QueryTestVariantFailureRateResponse.toJSON(response)),
  }, { overwriteRoutes: true });
};
