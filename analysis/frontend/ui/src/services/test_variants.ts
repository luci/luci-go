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

import { AuthorizedPrpcClient } from '@/clients/authorized_client';

import {
  Variant,
  Changelist,
} from './shared_models';

export const getTestVariantsService = () => {
  const client = new AuthorizedPrpcClient();
  return new TestVariantsService(client);
};

// A service to provide statistics about test variants.
export class TestVariantsService {
  private static SERVICE = 'luci.analysis.v1.TestVariants';

  client: AuthorizedPrpcClient;

  constructor(client: AuthorizedPrpcClient) {
    this.client = client;
  }

  // Queries the failure rate of specified test variants, returning
  // signals indicating if the test variant is flaky and/or
  // deterministically failing.
  async queryFailureRate(request: QueryTestVariantFailureRateRequest): Promise<QueryTestVariantFailureRateResponse> {
    return this.client.call(TestVariantsService.SERVICE, 'QueryFailureRate', request);
  }
}

export interface QueryTestVariantFailureRateRequest {
  // The LUCI Project for which test variants should be looked up.
  project: string;

  // The list of test variants to retrieve results for.
  // At most 100 test variants may be specified in one request.
  // It is an error to request the same test variant twice.
  testVariants: TestVariantIdentifier[];
}

export interface TestVariantIdentifier {
  // A unique identifier of the test in a LUCI project.
  testId: string;

  // Description of one specific way of running the test,
  // e.g. a specific bucket, builder and a test suite.
  variant?: Variant;

  // The variant hash. Alternative to specifying the variant.
  // Prefer to specify the full variant (if available), as the
  // variant hashing implementation is an implementation detail
  // and may change.
  variantHash?: string;
}

export interface QueryTestVariantFailureRateResponse {
  // The time buckets used for time interval data.
  intervals: QueryTestVariantFailureRateResponseInterval[];

  // The test variant failure rate analysis requested.
  // Test variants are returned in the order they were requested.
  testVariants?: TestVariantFailureRateAnalysis[];
}

// Interval defines the time buckets used for time interval
// data.
export interface QueryTestVariantFailureRateResponseInterval {
  // The interval being defined. age=1 is the most recent
  // interval, age=2 is the interval immediately before that,
  // and so on.
  intervalAge: number;

  // The start time of the interval (inclusive).
  // RFC 3339 encoded date/time.
  startTime: string;

  // The end time of the interval (exclusive).
  // RFC 3339 encoded date/time.
  endTime: string;
}

// Signals relevant to determining whether a test variant should be
// exonerated in presubmit.
export interface TestVariantFailureRateAnalysis {
  // A unique identifier of the test in a LUCI project.
  testId: string;

  // Description of one specific way of running the test,
  // e.g. a specific bucket, builder and a test suite.
  // Only populated if populated on the request.
  variant?: Variant;

  // The variant hash.
  // Only populated if populated on the request.
  variantHash?: string;

  // Statistics broken down by time interval. Intervals will be ordered
  // by recency, starting at the most recent interval (age = 1).
  //
  // The following filtering applies to verdicts used in time interval data:
  // - Verdicts are filtered to at most one per unique CL under test,
  //   with verdicts for multi-CL tryjob runs excluded.
  intervalStats: TestVariantFailureRateAnalysisIntervalStats[];

  // Examples of verdicts which had both expected and unexpected runs.
  //
  // Ordered by recency, starting at the most recent example at offset 0.
  //
  // Limited to at most 10. Further limited to only verdicts produced
  // since 5 weekdays ago (this corresponds to the exact same time range
  // as for which interval data is provided).
  runFlakyVerdictExamples?: TestVariantFailureRateAnalysisVerdictExample[];

  // The most recent verdicts for the test variant.
  //
  // The following filtering applies to verdicts used in this field:
  // - Verdicts are filtered to at most one per unique CL under test,
  //   with verdicts for multi-CL tryjob runs excluded.
  // - Verdicts for CLs authored by automation are excluded, to avoid a
  //   single repeatedly failing automatic uprev process populating
  //   this list with 10 failures.
  // Ordered by recency, starting at the most recent verdict at offset 0.
  //
  // Limited to at most 10. Further limited to only verdicts produced
  // since 5 weekdays ago (this corresponds to the exact same time range
  // as for which interval data is provided).
  recentVerdicts?: TestVariantFailureRateAnalysisRecentVerdict[];
}

export interface TestVariantFailureRateAnalysisIntervalStats {
  // The age of the interval. 1 is the most recent interval,
  // 2 is the interval immediately before that, and so on.
  // Cross reference with the intervals field on the
  // QueryTestVariantFailureRateResponse response to
  // identify the exact time interval this represents.
  intervalAge: number;

  // The number of verdicts which had only expected runs.
  // An expected run is a run (e.g. swarming task) which has at least
  // one expected result, excluding skipped results.
  totalRunExpectedVerdicts?: number;

  // The number of verdicts which had both expected and
  // unexpected runs.
  // An expected run is a run (e.g. swarming task) which has at least
  // one expected result, excluding skips.
  // An unexpected run is a run which had only unexpected
  // results (and at least one unexpected result), excluding skips.
  totalRunFlakyVerdicts?: number;

  // The number of verdicts which had only unexpected runs.
  // An unexpected run is a run (e.g. swarming task) which had only
  // unexpected results (and at least one unexpected result),
  // excluding skips.
  totalRunUnexpectedVerdicts?: number;
}

export interface TestVariantFailureRateAnalysisVerdictExample {
  // The partition time of the verdict. This the time associated with the
  // test result for test history purposes, usually the build or presubmit
  // run start time.
  // RFC 3339 encoded date/time.
  partitionTime: string;

  // The identity of the ingested invocation.
  ingestedInvocationId: string;

  // The changelist(s) tested, if any.
  changelists?: Changelist[];
}

export interface TestVariantFailureRateAnalysisRecentVerdict {
  // The partition time of the verdict. This the time associated with the
  // test result for test history purposes, usually the build or presubmit
  // run start time.
  // RFC 3339 encoded date/time.
  partitionTime: string;

  // The identity of the ingested invocation.
  ingestedInvocationId: string;

  // The changelist(s) tested, if any.
  changelists?: Changelist[];

  // Whether the verdict had an unexpected run.
  // An unexpected run is a run (e.g. swarming task) which
  // had only unexpected results, after excluding skips.
  //
  // Example: a verdict includes the result of two
  // swarming tasks (i.e. two runs), which each contain two
  // test results.
  // One of the two test runs has two unexpected failures.
  // Therefore, the verdict has an unexpected run.
  hasUnexpectedRuns?: boolean;
}
