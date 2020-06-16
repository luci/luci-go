// Copyright 2020 The LUCI Authors.
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

import { PrpcClient } from '@chopsui/prpc-client';

/**
 * Manually coded type definition and classes for resultdb service.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source: https://chromium.googlesource.com/infra/luci/luci-go/+/4525018bc0953bfa8597bd056f814dcf5e765142/resultdb/proto/rpc/v1/resultdb.proto
 */

export enum TestStatus {
  Unspecified = 'STATUS_UNSPECIFIED',
  Pass = 'PASS',
  Fail = 'FAIL',
  Crash = 'CRASH',
  Abort = 'ABORT',
  Skip = 'SKIP',
}

export enum InvocationState {
  Unspecified = 'STATE_UNSPECIFIED',
  Active = 'ACTIVE',
  Finalizing = 'FINALIZING',
  Finalized = 'FINALIZED',
}

export interface Invocation {
  readonly interrupted: boolean;
  readonly name: string;
  readonly state: InvocationState;
  readonly createTime: string;
  readonly finalizeTime: string;
  readonly deadline: string;
  readonly includedInvocations?: string[];
  readonly tags?: Tag[];
}

export interface TestResult {
  readonly name: string;
  readonly testId: string;
  readonly resultId: string;
  readonly variant?: Variant;
  readonly expected?: boolean;
  readonly status: TestStatus;
  readonly summaryHtml: string;
  readonly startTime: string;
  readonly duration: string;
  readonly tags?: Tag[];
}

export interface TestExoneration {
  readonly name: string;
  readonly testId: string;
  readonly variant?: Variant;
  readonly exonerationId: string;
  readonly explanationHTML?: string;
}

export interface Artifact {
  readonly name: string;
  readonly artifactId: string;
  readonly fetchUrl?: string;
  readonly fetchUrlExpiration?: string;
  readonly contentType: string;
  readonly sizeBytes: number;
}

export interface Variant {
  readonly def: {[key: string]: string};
}

export interface Tag {
  readonly key: string;
  readonly value: string;
}

export interface GetInvocationRequest {
  readonly name: string;
}

export interface QueryTestResultRequest {
  readonly invocations: string[];
  readonly predicate?: TestResultPredicate;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface QueryTestExonerationsRequest {
  readonly invocations: string[];
  readonly predicate?: TestExonerationPredicate;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface ListArtifactsRequest {
  readonly parent: string;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface GetArtifactRequest {
  readonly name: string;
}

export interface TestResultPredicate {
  readonly testIdRegexp?: string;
  readonly variant?: VariantPredicate;
  readonly expectancy?: Expectancy;
}

export interface TestExonerationPredicate {
  readonly testIdRegexp?: string;
  readonly variant?: VariantPredicate;
}

export type VariantPredicate = { readonly equals: Variant; } | { readonly contains: Variant; };

export const enum Expectancy {
  All = 'ALL',
  VariantsWithUnexpectedResults = 'VARIANTS_WITH_UNEXPECTED_RESULTS',
}

export interface QueryTestResultsResponse {
  readonly testResults?: TestResult[];
  readonly nextPageToken?: string;
}

export interface QueryTestExonerationsResponse {
  readonly testExonerations?: TestExoneration[];
  readonly nextPageToken?: string;
}

export interface ListArtifactsResponse {
  readonly artifacts?: Artifact[];
  readonly nextPageToken?: string;
}

const SERVICE = 'luci.resultdb.v1.ResultDB';

export class ResultDb {
  private prpcClient: PrpcClient;

  constructor(readonly host: string, accessToken: string) {
    this.prpcClient = new PrpcClient({host, accessToken});
  }

  async getInvocation(req: GetInvocationRequest): Promise<Invocation> {
    return await this.call(
      'GetInvocation',
      req,
    ) as Invocation;
  }

  async queryTestResults(req: QueryTestResultRequest) {
    return await this.call(
        'QueryTestResults',
        req,
    ) as QueryTestResultsResponse;
  }

  async queryTestExonerations(req: QueryTestExonerationsRequest) {
    return await this.call(
        'QueryTestExonerations',
        req,
    ) as QueryTestExonerationsResponse;
  }

  async listArtifacts(req: ListArtifactsRequest) {
    return await this.call(
      'ListArtifacts',
      req,
    ) as ListArtifactsResponse;
  }

  async getArtifact(req: GetArtifactRequest) {
    return await this.call(
      'GetArtifact',
      req,
    ) as Artifact;
  }

  private call(method: string, message: object) {
    return this.prpcClient.call(
      SERVICE,
      method,
      message,
    );
  }
}
