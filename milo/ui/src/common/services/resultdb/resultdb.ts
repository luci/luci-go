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

import stableStringify from 'fast-json-stable-stringify';

import { BuilderID } from '@/common/services/buildbucket';
import {
  GerritChange,
  GitilesCommit,
  StringPair,
} from '@/common/services/common';
import { logging } from '@/common/tools/logging';
import { cached, CacheOption } from '@/generic_libs/tools/cached_fn';
import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';
import { sha256 } from '@/generic_libs/tools/utils';
import { ToString } from '@/generic_libs/types';

/* eslint-disable max-len */
/**
 * Manually coded type definition and classes for resultdb service.
 * TODO(weiweilin): To be replaced by code generated version once we have one.
 * source: https://chromium.googlesource.com/infra/luci/luci-go/+/4525018bc0953bfa8597bd056f814dcf5e765142/resultdb/proto/rpc/v1/resultdb.proto
 */
/* eslint-enable max-len */

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
  readonly realm: string;
  readonly state: InvocationState;
  readonly createTime: string;
  readonly finalizeTime: string;
  readonly deadline: string;
  readonly includedInvocations?: string[];
  readonly tags?: StringPair[];
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
  readonly duration?: string;
  readonly tags?: StringPair[];
  readonly failureReason?: FailureReason;
}

export interface TestLocation {
  readonly repo: string;
  readonly fileName: string;
  readonly line?: number;
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
  readonly fetchUrl: string;
  readonly fetchUrlExpiration: string;
  readonly contentType: string;
  readonly sizeBytes: number;
}

export type TestVariantDef = { [key: string]: string };

export interface Variant {
  readonly def: TestVariantDef;
}

export interface FailureReason {
  readonly primaryErrorMessage: string;
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

export type VariantPredicate =
  | { readonly equals: Variant }
  | { readonly contains: Variant };

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
  readonly resultLimit?: number;
}

export interface QueryTestVariantsResponse {
  readonly testVariants?: readonly TestVariant[];
  readonly nextPageToken?: string;
}

export interface TestVariant {
  readonly testId: string;
  readonly variant?: Variant;
  readonly variantHash: string;
  readonly status: TestVariantStatus;
  readonly results?: readonly TestResultBundle[];
  readonly exonerations?: readonly TestExoneration[];
  readonly testMetadata?: TestMetadata;
  readonly sourcesId: string;
  readonly instruction?: VerdictInstruction;
}

export interface Sources {
  readonly gitilesCommit?: GitilesCommit;
  readonly changelists?: GerritChange[];
}

export const enum TestVariantStatus {
  TEST_VARIANT_STATUS_UNSPECIFIED = 'TEST_VARIANT_STATUS_UNSPECIFIED',
  UNEXPECTED = 'UNEXPECTED',
  UNEXPECTEDLY_SKIPPED = 'UNEXPECTEDLY_SKIPPED',
  FLAKY = 'FLAKY',
  EXONERATED = 'EXONERATED',
  EXPECTED = 'EXPECTED',
}

// Note: once we have more than 9 statuses, we need to add '0' prefix so '10'
// won't appear before '2' after sorting.
export const TEST_VARIANT_STATUS_CMP_STRING = {
  [TestVariantStatus.TEST_VARIANT_STATUS_UNSPECIFIED]: '0',
  [TestVariantStatus.UNEXPECTED]: '1',
  [TestVariantStatus.UNEXPECTEDLY_SKIPPED]: '2',
  [TestVariantStatus.FLAKY]: '3',
  [TestVariantStatus.EXONERATED]: '4',
  [TestVariantStatus.EXPECTED]: '5',
};

export interface TestMetadata {
  readonly name?: string;
  readonly location?: TestLocation;
  readonly propertiesSchema?: string;
  readonly properties?: { [key: string]: unknown };
}

export interface TestResultBundle {
  readonly result: TestResult;
}

export interface TestVariantIdentifier {
  readonly testId: string;
  readonly variantHash: string;
}

export interface BatchGetTestVariantsRequest {
  readonly invocation: string;
  readonly testVariants: readonly TestVariantIdentifier[];
  readonly resultLimit?: number;
}

export interface BatchGetTestVariantsResponse {
  readonly testVariants?: readonly TestVariant[];
  readonly sources: { [key: string]: Sources };
}

export interface QueryTestMetadataRequest {
  readonly project: string;
  readonly predicate?: TestMetadataPredicate;
  readonly pageSize?: number;
  readonly pageToken?: string;
}

export interface TestMetadataPredicate {
  readonly testIds?: string[];
}

export interface QueryTestMetadataResponse {
  readonly testMetadata?: TestMetadataDetail[];
  readonly nextPageToken?: string;
}

export interface TestMetadataDetail {
  readonly name: string;
  readonly project: string;
  readonly testId: string;
  readonly refHash: string;
  readonly sourceRef: SourceRef;
  readonly testMetadata?: TestMetadata;
}

export type SourceRef = { readonly gitiles?: GitilesRef };
export interface GitilesRef {
  readonly host: string;
  readonly project: string;
  readonly ref: string;
}

export interface VerdictInstruction {
  readonly instruction: string;
}

// The maximum number of results that can be included in a test variant returned
// from the RPC.
export const RESULT_LIMIT = 100;

export class ResultDb {
  static readonly SERVICE = 'luci.resultdb.v1.ResultDB';

  private readonly cachedCallFn: (
    opt: CacheOption,
    method: string,
    message: object,
  ) => Promise<unknown>;

  constructor(client: PrpcClientExt) {
    this.cachedCallFn = cached(
      (method: string, message: object) =>
        client.call(ResultDb.SERVICE, method, message),
      {
        key: (method, message) => `${method}-${stableStringify(message)}`,
      },
    );
  }

  async getInvocation(
    req: GetInvocationRequest,
    cacheOpt: CacheOption = {},
  ): Promise<Invocation> {
    return (await this.cachedCallFn(
      cacheOpt,
      'GetInvocation',
      req,
    )) as Invocation;
  }

  async queryTestResults(
    req: QueryTestResultsRequest,
    cacheOpt: CacheOption = {},
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryTestResults',
      req,
    )) as QueryTestResultsResponse;
  }

  async queryTestExonerations(
    req: QueryTestExonerationsRequest,
    cacheOpt: CacheOption = {},
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryTestExonerations',
      req,
    )) as QueryTestExonerationsResponse;
  }

  async listArtifacts(req: ListArtifactsRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(
      cacheOpt,
      'ListArtifacts',
      req,
    )) as ListArtifactsResponse;
  }

  async queryArtifacts(req: QueryArtifactsRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryArtifacts',
      req,
    )) as QueryArtifactsResponse;
  }

  async getArtifact(req: GetArtifactRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(cacheOpt, 'GetArtifact', req)) as Artifact;
  }

  async queryTestVariants(
    req: QueryTestVariantsRequest,
    cacheOpt: CacheOption = {},
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryTestVariants',
      req,
    )) as QueryTestVariantsResponse;
  }

  async batchGetTestVariants(
    req: BatchGetTestVariantsRequest,
    cacheOpt: CacheOption = {},
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'BatchGetTestVariants',
      req,
    )) as BatchGetTestVariantsResponse;
  }

  async queryTestMetadata(
    req: QueryTestMetadataRequest,
    cacheOpt: CacheOption = {},
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryTestMetadata',
      req,
    )) as QueryTestMetadataResponse;
  }
}

export interface TestResultIdentifier {
  readonly invocationId: string;
  readonly testId: string;
  readonly resultId: string;
}

/**
 * Parses the artifact name and get the individual components.
 */
export function parseArtifactName(artifactName: string): ArtifactIdentifier {
  const match = artifactName.match(
    /^invocations\/(.*?)\/(?:tests\/(.*?)\/results\/(.*?)\/)?artifacts\/(.*)$/,
  );
  if (!match) {
    throw new Error(`invalid artifact name: ${artifactName}`);
  }

  const [, invocationId, testId, resultId, artifactId] = match as string[];

  return {
    invocationId,
    testId: testId ? decodeURIComponent(testId) : undefined,
    resultId: resultId ? resultId : undefined,
    artifactId,
  };
}

export type ArtifactIdentifier =
  | InvocationArtifactIdentifier
  | TestResultArtifactIdentifier;

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
    return `invocations/${identifier.invocationId}/tests/${encodeURIComponent(
      identifier.testId,
    )}/results/${identifier.resultId}/artifacts/${identifier.artifactId}`;
  } else {
    return `invocations/${identifier.invocationId}/artifacts/${identifier.artifactId}`;
  }
}

/**
 * Computes invocation ID for the build from the given build ID.
 */
export function getInvIdFromBuildId(buildId: string): string {
  return `build-${buildId}`;
}

/**
 * Computes invocation ID for the build from the given builder ID and build number.
 */
export async function getInvIdFromBuildNum(
  builder: BuilderID,
  buildNum: number,
): Promise<string> {
  const builderId = `${builder.project}/${builder.bucket}/${builder.builder}`;
  return `build-${await sha256(builderId)}-${buildNum}`;
}

/**
 * Create a test variant property getter for the given property key.
 *
 * A property key must be one of the following:
 * 1. 'status': status of the test variant.
 * 2. 'name': test_metadata.name of the test variant.
 * 3. 'v.{variant_key}': variant.def[variant_key] of the test variant (e.g.
 * v.gpu).
 */
export function createTVPropGetter(
  propKey: string,
): (v: TestVariant) => ToString {
  if (propKey.match(/^v[.]/i)) {
    const variantKey = propKey.slice(2);
    return (v) => v.variant?.def[variantKey] || '';
  }
  propKey = propKey.toLowerCase();
  switch (propKey) {
    case 'name':
      return (v) => v.testMetadata?.name || v.testId;
    case 'status':
      return (v) => v.status;
    default:
      logging.warn('invalid property key', propKey);
      return () => '';
  }
}

/**
 * Create a test variant compare function for the given sorting key list.
 *
 * A sorting key must be one of the following:
 * 1. '{property_key}': sort by property_key in ascending order.
 * 2. '-{property_key}': sort by property_key in descending order.
 */
export function createTVCmpFn(
  sortingKeys: readonly string[],
): (v1: TestVariant, v2: TestVariant) => number {
  const sorters: Array<[number, (v: TestVariant) => { toString(): string }]> =
    sortingKeys.map((key) => {
      const [mul, propKey] = key.startsWith('-')
        ? [-1, key.slice(1)]
        : [1, key];
      const propGetter = createTVPropGetter(propKey);

      // Status should be be sorted by their significance not by their string
      // representation.
      if (propKey.toLowerCase() === 'status') {
        return [
          mul,
          (v) =>
            TEST_VARIANT_STATUS_CMP_STRING[propGetter(v) as TestVariantStatus],
        ];
      }
      return [mul, propGetter];
    });
  return (v1, v2) => {
    for (const [mul, propGetter] of sorters) {
      const cmp =
        propGetter(v1).toString().localeCompare(propGetter(v2).toString()) *
        mul;
      if (cmp !== 0) {
        return cmp;
      }
    }
    return 0;
  };
}

/**
 * Computes the display label for a given property key.
 */
export function getPropKeyLabel(key: string) {
  // If the key has the format of '{type}.{value}', hide the '{type}.' prefix.
  // It's safe to cast here because the 2nd capture group must match something.
  // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
  return key.match(/^([^.]*\.)?(.*)$/)![2];
}
