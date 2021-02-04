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
import { comparer } from 'mobx';
import { createTransformer, fromPromise, FULFILLED } from 'mobx-utils';

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
  readonly variantHash?: string;
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
  readonly variantHash?: string;
  readonly exonerationId: string;
  readonly explanationHtml?: string;
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

export interface QueryTestResultsRequest {
  readonly invocations: string[];
  readonly readMask?: string;
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

export interface EdgeTypeSet {
  readonly includedInvocations: boolean;
  readonly testResults: boolean;
}

export interface QueryArtifactsRequest {
  readonly invocations: string[];
  readonly followEdges?: EdgeTypeSet;
  readonly testResultPredicate?: TestResultPredicate;
  readonly maxStaleness?: string;
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

export interface QueryArtifactsResponse {
  readonly artifacts?: Artifact[];
  readonly nextPageToken?: string;
}

export interface QueryTestVariantsRequest {
  readonly invocations: readonly string[];
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface QueryTestVariantsResponse {
  readonly testVariants: readonly TestVariant[];
  readonly nextPageToken?: string;
}

export interface TestVariant {
  readonly testId: string;
  readonly variant?: Variant;
  readonly variantHash: string;
  readonly status: TestVariantStatus;
  readonly results?: readonly TestResultBundle[];
  readonly exonerations?: readonly TestExoneration[];
}

export const enum TestVariantStatus {
  TEST_VARIANT_STATUS_UNSPECIFIED = 'TEST_VARIANT_STATUS_UNSPECIFIED',
  UNEXPECTED = 'UNEXPECTED',
  FLAKY = 'FLAKY',
  EXONERATED = 'EXONERATED',
  EXPECTED = 'EXPECTED',
}

export interface TestResultBundle {
  readonly result: TestResult;
}

export class ResultDb {
  private static SERVICE = 'luci.resultdb.v1.ResultDB';
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

  async queryTestResults(req: QueryTestResultsRequest) {
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

  async queryArtifacts(req: QueryArtifactsRequest) {
    return await this.call(
      'QueryArtifacts',
      req,
    ) as QueryArtifactsResponse;
  }

  async getArtifact(req: GetArtifactRequest) {
    return await this.call(
      'GetArtifact',
      req,
    ) as Artifact;
  }

  /**
   * Returns the cached list artifacts response of an invocation.
   */
  private getListArtifactsResOfInv = createTransformer((invName: string) => {
    const artifacts = this.listArtifacts({parent: invName});
    return fromPromise(artifacts);
  });

  /**
   * Returns the cached artifacts of an invocation.
   * If the artifacts are not cached yet,
   * 1. return null, and
   * 2. fetch the artifacts, and
   * 3. once the artifacts are fetched, notifies the subscribers with the new
   * artifacts
   *
   * @param invName: Invocation Name.
   * @return artifacts of the invocation (if cached) or null.
   */
  getCachedArtifactsOfInv = createTransformer(
    (invName: string) => {
      const listArtifactRes = this.getListArtifactsResOfInv(invName);
      return listArtifactRes.state === FULFILLED
        ? listArtifactRes.value.artifacts || []
        : null;
    },
    {
      equals: comparer.shallow,
    },
  );

  private call(method: string, message: object) {
    return this.prpcClient.call(
      ResultDb.SERVICE,
      method,
      message,
    );
  }
}

export class UISpecificService {
  private prpcClient: PrpcClient;
  private static SERVICE = 'luci.resultdb.internal.ui.UI';

  constructor(readonly host: string, accessToken: string) {
    this.prpcClient = new PrpcClient({host, accessToken});
  }

  async queryTestVariants(req: QueryTestVariantsRequest) {
    return await this.call(
      'QueryTestVariants',
      req,
    ) as QueryTestVariantsResponse;
  }

  private call(method: string, message: object) {
    return this.prpcClient.call(
      UISpecificService.SERVICE,
      method,
      message,
    );
  }
}

/**
 * Parses the artifact name and get the individual components.
 */
export function parseArtifactName(artifactName: string): ArtifactIdentifier {
  const match = artifactName
    .match(/invocations\/(.*?)\/(?:tests\/(.*?)\/results\/(.*?)\/)?artifacts\/(.*)/)!;

  const [, invocationId, testId, resultId, artifactId] = match;

  return {
    invocationId,
    testId: testId ? decodeURIComponent(testId) : undefined,
    resultId: resultId ? resultId : undefined,
    artifactId,
  };
}

export type ArtifactIdentifier = InvocationArtifactIdentifier | TestResultArtifactIdentifier;

export interface InvocationArtifactIdentifier {
  readonly invocationId: string;
  readonly testId?: string;
  readonly resultId?: string;
  readonly artifactId: string;
}

export interface TestResultArtifactIdentifier {
  readonly invocationId: string;
  readonly testId: string;
  readonly resultId: string;
  readonly artifactId: string;
}

/**
 * Constructs the name of the artifact.
 */
export function constructArtifactName(identifier: ArtifactIdentifier) {
  if (identifier.testId && identifier.resultId) {
    return `invocations/${identifier.invocationId}/tests/${encodeURIComponent(identifier.testId)}/results/${identifier.resultId}/artifacts/${identifier.artifactId}`;
  } else {
    return `invocations/${identifier.invocationId}/artifacts/${identifier.artifactId}`;
  }
}
