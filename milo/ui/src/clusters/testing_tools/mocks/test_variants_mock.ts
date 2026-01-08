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

import {
  QueryTestVariantFailureRateRequest,
  QueryTestVariantFailureRateRequest_TestVariant,
  QueryTestVariantFailureRateResponse,
  QueryTestVariantStabilityRequest,
  QueryTestVariantStabilityRequest_TestVariantPosition,
  QueryTestVariantStabilityResponse,
  TestStabilityCriteria,
  TestVariantFailureRateAnalysis,
  TestVariantStabilityAnalysis,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_variants.pb';
import { mockFetchRaw } from '@/testing_tools/jest_utils';

export const getMockTestVariantIdentifier = (
  id: string,
): QueryTestVariantFailureRateRequest_TestVariant => {
  return QueryTestVariantFailureRateRequest_TestVariant.create({
    testId: id,
  });
};

export const getMockTestVariantFailureRateAnalysis = (
  id: string,
): TestVariantFailureRateAnalysis => {
  return TestVariantFailureRateAnalysis.create({
    testId: id,
    intervalStats: Object.freeze([
      {
        intervalAge: 1,
        totalRunExpectedVerdicts: 101,
        totalRunFlakyVerdicts: 102,
        totalRunUnexpectedVerdicts: 103,
      },
      {
        intervalAge: 2,
        totalRunExpectedVerdicts: 0,
        totalRunFlakyVerdicts: 0,
        totalRunUnexpectedVerdicts: 0,
      },
      {
        intervalAge: 3,
        totalRunExpectedVerdicts: 0,
        totalRunFlakyVerdicts: 0,
        totalRunUnexpectedVerdicts: 0,
      },
      {
        intervalAge: 4,
        totalRunExpectedVerdicts: 0,
        totalRunFlakyVerdicts: 0,
        totalRunUnexpectedVerdicts: 0,
      },
      {
        intervalAge: 5,
        totalRunExpectedVerdicts: 0,
        totalRunFlakyVerdicts: 0,
        totalRunUnexpectedVerdicts: 0,
      },
    ]),
  });
};

export const getMockTestVariantPosition = (
  id: string,
): QueryTestVariantStabilityRequest_TestVariantPosition => {
  return QueryTestVariantStabilityRequest_TestVariantPosition.create({
    testId: id,
    sources: {
      gitilesCommit: {
        host: 'myproject.googlesource.com',
        project: 'myproject/src',
        ref: 'refs/heads/mybranch',
        commitHash: 'ff'.repeat(20),
        position: '999999999999',
      },
    },
  });
};

export const getMockTestVariantStabilityAnalysis = (
  id: string,
): TestVariantStabilityAnalysis => {
  return TestVariantStabilityAnalysis.create({
    testId: id,
    failureRate: {
      isMet: true,
      unexpectedTestRuns: 101,
      consecutiveUnexpectedTestRuns: 102,
    },
    flakeRate: {
      isMet: false,
      runFlakyVerdicts: 201,
      totalVerdicts: 202,
      startPosition: '301',
      endPosition: '302',
    },
  });
};

export const getMockTestStabilityCriteria = (): TestStabilityCriteria => {
  return TestStabilityCriteria.create({
    failureRate: {
      failureThreshold: 6,
      consecutiveFailureThreshold: 3,
    },
    flakeRate: {
      minWindow: 100,
      flakeRateThreshold: 0.01,
      flakeThreshold: 2,
    },
  });
};

export const mockQueryFailureRate = (
  request: QueryTestVariantFailureRateRequest,
  response: QueryTestVariantFailureRateResponse,
) => {
  mockFetchRaw(
    (url, init) => {
      if (
        url !==
        'https://staging.analysis.api.luci.app/prpc/luci.analysis.v1.TestVariants/QueryFailureRate'
      ) {
        return false;
      }
      if (!init?.body || typeof init.body !== 'string') {
        return false;
      }
      return (
        init.body ===
        JSON.stringify(QueryTestVariantFailureRateRequest.toJSON(request))
      );
    },
    ")]}'\n" +
      JSON.stringify(QueryTestVariantFailureRateResponse.toJSON(response)),
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
    },
  );
};

export const mockQueryStability = (
  request: QueryTestVariantStabilityRequest,
  response: QueryTestVariantStabilityResponse,
) => {
  mockFetchRaw(
    (url, init) => {
      if (
        url !==
        'https://staging.analysis.api.luci.app/prpc/luci.analysis.v1.TestVariants/QueryStability'
      ) {
        return false;
      }
      if (!init?.body || typeof init.body !== 'string') {
        return false;
      }
      return (
        init.body ===
        JSON.stringify(QueryTestVariantStabilityRequest.toJSON(request))
      );
    },
    ")]}'\n" +
      JSON.stringify(QueryTestVariantStabilityResponse.toJSON(response)),
    {
      headers: {
        'X-Prpc-Grpc-Code': '0',
      },
    },
  );
};
