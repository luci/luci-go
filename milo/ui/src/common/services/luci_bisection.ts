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

import stableStringify from 'fast-json-stable-stringify';

import { CacheOption, cached } from '@/generic_libs/tools/cached_fn';
import { PrpcClientExt } from '@/generic_libs/tools/prpc_client_ext';

import { Variant } from './resultdb';

export type AnalysisStatus =
  | 'ANALYSIS_STATUS_UNSPECIFIED'
  | 'CREATED'
  | 'RUNNING'
  | 'FOUND'
  | 'NOTFOUND'
  | 'ERROR'
  | 'SUSPECTFOUND'
  | 'UNSUPPORTED'
  | 'DISABLED';

export type AnalysisRunStatus =
  | 'ANALYSIS_RUN_STATUS_UNSPECIFIED'
  | 'STARTED'
  | 'ENDED'
  | 'CANCELED';

export type SuspectConfidenceLevel =
  | 'SUSPECT_CONFIDENCE_LEVEL_UNSPECIFIED'
  | 'LOW'
  | 'MEDIUM'
  | 'HIGH';

export type CulpritActionType =
  | 'CULPRIT_ACTION_TYPE_UNSPECIFIED'
  | 'NO_ACTION'
  | 'CULPRIT_AUTO_REVERTED'
  | 'REVERT_CL_CREATED'
  | 'CULPRIT_CL_COMMENTED'
  | 'BUG_COMMENTED'
  | 'EXISTING_REVERT_CL_COMMENTED';

export type CulpritInactionReason =
  | 'CULPRIT_INACTION_REASON_UNSPECIFIED'
  | 'REVERTED_BY_BISECTION'
  | 'REVERTED_MANUALLY'
  | 'REVERT_OWNED_BY_BISECTION'
  | 'REVERT_HAS_COMMENT'
  | 'CULPRIT_HAS_COMMENT'
  | 'ANALYSIS_CANCELED'
  | 'ACTIONS_DISABLED';

export type BuildFailureType =
  | 'BUILD_FAILURE_TYPE_UNSPECIFIED'
  | 'COMPILE'
  | 'TEST'
  | 'INFRA'
  | 'OTHER';

export type RerunStatus =
  | 'RERUN_STATUS_UNSPECIFIED'
  | 'RERUN_STATUS_IN_PROGRESS'
  | 'RERUN_STATUS_PASSED'
  | 'RERUN_STATUS_FAILED'
  | 'RERUN_STATUS_INFRA_FAILED'
  | 'RERUN_STATUS_CANCELED'
  | 'RERUN_STATUS_TEST_SKIPPED';

export function isAnalysisComplete(status: AnalysisStatus) {
  const completeStatuses: Array<AnalysisStatus> = [
    'FOUND',
    'NOTFOUND',
    'ERROR',
    'SUSPECTFOUND',
  ];
  return completeStatuses.includes(status);
}

export interface QueryAnalysisRequest {
  buildFailure: BuildFailure;
}

export interface QueryAnalysisResponse {
  analyses: Analysis[];
}

export interface ListAnalysesRequest {
  pageSize?: number;
  pageToken?: string;
}

export interface ListAnalysesResponse {
  analyses: Analysis[];
  nextPageToken: string;
}

export interface Analysis {
  analysisId: string;
  status: AnalysisStatus;
  runStatus: AnalysisRunStatus;
  lastPassedBbid: string;
  firstFailedBbid: string;
  createdTime: string;
  lastUpdatedTime: string;
  endTime: string;
  heuristicResult?: HeuristicAnalysisResult;
  nthSectionResult?: NthSectionAnalysisResult;
  builder?: BuilderID;
  buildFailureType: BuildFailureType;
  culprits?: Culprit[];
}

export interface BuildFailure {
  bbid: string;
  failedStepName: string;
}

export interface BuilderID {
  project: string;
  bucket: string;
  builder: string;
}

export interface HeuristicAnalysisResult {
  status: AnalysisStatus;
  suspects?: HeuristicSuspect[];
}

export interface HeuristicSuspect {
  gitilesCommit: GitilesCommit;
  reviewUrl: string;
  reviewTitle?: string;
  score: string;
  justification: string;
  confidenceLevel: SuspectConfidenceLevel;
  verificationDetails: SuspectVerificationDetails;
}

export interface NthSectionAnalysisResult {
  status: AnalysisStatus;
  startTime: string;
  endTime: string;
  suspect?: Suspect;
  remainingNthSectionRange?: RegressionRange;
  reruns: SingleRerun[];
  blameList: BlameList;
}

export interface Suspect {
  commit: GitilesCommit;
  reviewUrl: string;
  reviewTitle?: string;
  verificationDetails: SuspectVerificationDetails;
  type?: string;
}

export interface BlameList {
  commits: BlameListSingleCommit[];
}

export interface BlameListSingleCommit {
  commit: string;
  reviewUrl: string;
  reviewTitle: string;
}

export interface GitilesCommit {
  host: string;
  project: string;
  id: string;
  ref: string;
  position?: string;
}

export interface RegressionRange {
  lastPassed: GitilesCommit;
  firstFailed: GitilesCommit;
}

export interface Culprit {
  commit: GitilesCommit;
  reviewUrl: string;
  reviewTitle?: string;
  culpritAction?: CulpritAction[];
  verificationDetails: SuspectVerificationDetails;
}

export interface CulpritAction {
  actionType: CulpritActionType;
  revertClUrl?: string;
  bugUrl?: string;
  inactionReason?: CulpritInactionReason;
}

export interface SuspectVerificationDetails {
  status: string;
  suspectRerun?: SingleRerun;
  parentRerun?: SingleRerun;
}

export interface SingleRerun {
  startTime: string;
  endTime: string;
  bbid: string;
  rerunResult: RerunResult;
  commit: GitilesCommit;
  index?: string;
  type: string;
}

export interface RerunResult {
  rerunStatus: RerunStatus;
}

export interface CL {
  commitID: string;
  title: string;
  reviewURL: string;
}

export interface PrimeSuspect {
  cl: CL;
  culpritStatus: string;
  accuseSource: string;
}

export interface ListTestAnalysesRequest {
  project: string;
  pageSize?: number;
  pageToken?: string;
  fields?: FieldMask;
}

export interface FieldMask {
  paths: string[];
}

export interface ListTestAnalysesResponse {
  analyses: TestAnalysis[];
  nextPageToken?: string;
}

export interface GetTestAnalysisRequest {
  analysisId: number;
}

export interface TestAnalysis {
  analysisId: string;
  status: AnalysisStatus;
  runStatus: AnalysisRunStatus;
  createdTime: string;
  endTime?: string;
  nthSectionResult?: NthSectionAnalysisResult;
  builder: BuilderID;
  testFailures: TestFailure[];
  culprit?: Culprit;
  sampleBbid: number;
}

export interface TestFailure {
  testId: string;
  variant: Variant;
  variantHash: string;
  isDiverged: boolean;
  isPrimary: boolean;
  startHour: string;
}

// A service to handle LUCI Bisection-related pRPC requests.
export class LUCIBisectionService {
  // The name of the pRPC service to connect to.
  static readonly SERVICE = 'luci.bisection.v1.Analyses';
  private readonly cachedCallFn: (
    opt: CacheOption,
    method: string,
    message: object,
  ) => Promise<unknown>;

  constructor(client: PrpcClientExt) {
    this.cachedCallFn = cached(
      (method: string, message: object) =>
        client.call(LUCIBisectionService.SERVICE, method, message),
      {
        key: (method, message) => `${method}-${stableStringify(message)}`,
      },
    );
  }

  async queryAnalysis(req: QueryAnalysisRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(
      cacheOpt,
      'QueryAnalysis',
      req,
    )) as QueryAnalysisResponse;
  }

  async listAnalyses(req: ListAnalysesRequest, cacheOpt: CacheOption = {}) {
    return (await this.cachedCallFn(
      cacheOpt,
      'ListAnalyses',
      req,
    )) as ListAnalysesResponse;
  }

  async listTestAnalyses(
    req: ListTestAnalysesRequest,
    cacheOpt: CacheOption = {},
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'ListTestAnalyses',
      req,
    )) as ListTestAnalysesResponse;
  }

  async getTestAnalysis(
    req: GetTestAnalysisRequest,
    cacheOpt: CacheOption = {},
  ) {
    return (await this.cachedCallFn(
      cacheOpt,
      'GetTestAnalysis',
      req,
    )) as TestAnalysis;
  }
}
