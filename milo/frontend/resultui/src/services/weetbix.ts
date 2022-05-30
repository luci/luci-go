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

import stableStringify from 'fast-json-stable-stringify';

import { cached, CacheOption } from '../libs/cached_fn';
import { PrpcClientExt } from '../libs/prpc_client_ext';

export interface Variant {
  readonly def: { [key: string]: string };
}

export type VariantPredicate = { readonly equals: Variant } | { readonly contains: Variant };

export const enum SubmittedFilter {
  SUBMITTED_FILTER_UNSPECIFIED = 'SUBMITTED_FILTER_UNSPECIFIED',
  ONLY_SUBMITTED = 'ONLY_SUBMITTED',
  ONLY_UNSUBMITTED = 'ONLY_UNSUBMITTED',
}

export enum TestVerdictStatus {
  TEST_VERDICT_STATUS_UNSPECIFIED = 'TEST_VERDICT_STATUS_UNSPECIFIED',
  UNEXPECTED = 'UNEXPECTED',
  UNEXPECTEDLY_SKIPPED = 'UNEXPECTEDLY_SKIPPED',
  FLAKY = 'FLAKY',
  EXONERATED = 'EXONERATED',
  EXPECTED = 'EXPECTED',
}

export interface TimeRange {
  readonly earliest?: string;
  readonly latest?: string;
}

export interface TestVerdictPredicate {
  readonly subRealm?: string;
  readonly variantPredicate?: VariantPredicate;
  readonly submittedFilter?: SubmittedFilter;
  readonly partitionTimeRange?: TimeRange;
}

export interface QueryTestHistoryRequest {
  readonly project: string;
  readonly testId: string;
  readonly predicate: TestVerdictPredicate;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface TestVerdict {
  readonly testId: string;
  readonly variantHash: string;
  readonly invocationId: string;
  readonly status: TestVerdictStatus;
  readonly partitionTime: string;
  readonly passedAvgDuration: string;
}

export interface QueryTestHistoryResponse {
  readonly verdicts?: readonly TestVerdict[];
  readonly nextPageToken?: string;
}

export interface QueryTestHistoryStatsRequest {
  readonly project: string;
  readonly testId: string;
  readonly predicate: TestVerdictPredicate;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface QueryTestHistoryStatsResponseGroup {
  readonly partitionTime: string;
  readonly variantHash: string;
  readonly unexpectedCount?: number;
  readonly unexpectedlySkippedCount?: number;
  readonly flakyCount?: number;
  readonly exoneratedCount?: number;
  readonly expectedCount?: number;
  readonly passedAvgDuration?: string;
}

export interface QueryTestHistoryStatsResponse {
  readonly groups?: readonly QueryTestHistoryStatsResponseGroup[];
  readonly nextPageToken?: string;
}

export interface QueryVariantsRequest {
  readonly project: string;
  readonly testId: string;
  readonly subRealm?: string;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface QueryVariantsResponseVariantInfo {
  readonly variantHash: string;
  readonly variant: Variant;
}

export interface QueryVariantsResponse {
  readonly variants?: readonly QueryVariantsResponseVariantInfo[];
  readonly nextPageToken?: string;
}

export class TestHistoryService {
  private static SERVICE = 'weetbix.v1.TestHistory';

  private readonly cachedCallFn: (opt: CacheOption, method: string, message: object) => Promise<unknown>;

  constructor(client: PrpcClientExt) {
    this.cachedCallFn = cached(
      (method: string, message: object) => client.call(TestHistoryService.SERVICE, method, message),
      {
        key: (method, message) => `${method}-${stableStringify(message)}`,
      }
    );
  }

  async query(req: QueryTestHistoryRequest, cacheOpt: CacheOption = {}): Promise<QueryTestHistoryResponse> {
    return (await this.cachedCallFn(cacheOpt, 'Query', req)) as QueryTestHistoryResponse;
  }

  async queryStats(
    req: QueryTestHistoryStatsRequest,
    cacheOpt: CacheOption = {}
  ): Promise<QueryTestHistoryStatsResponse> {
    return (await this.cachedCallFn(cacheOpt, 'QueryStats', req)) as QueryTestHistoryStatsResponse;
  }

  async queryVariants(req: QueryVariantsRequest, cacheOpt: CacheOption = {}): Promise<QueryVariantsResponse> {
    return (await this.cachedCallFn(cacheOpt, 'QueryVariants', req)) as QueryVariantsResponse;
  }
}
