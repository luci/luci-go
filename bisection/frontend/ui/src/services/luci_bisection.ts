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


import { AuthorizedPrpcClient } from '../clients/authorized_client';

export const getLUCIBisectionService = () => {
  const client = new AuthorizedPrpcClient();
  return new LUCIBisectionService(client);
};

// A service to handle LUCI Bisection-related pRPC requests.
export class LUCIBisectionService {
  // The name of the pRPC service to connect to.
  // TODO: update this once the pRPC service is renamed
  //       from GoFindit to LUCI Bisection
  private static SERVICE = 'gofindit.GoFinditService';

  client: AuthorizedPrpcClient;

  constructor(client: AuthorizedPrpcClient) {
    this.client = client;
  }

  async queryAnalysis(
    request: QueryAnalysisRequest
  ): Promise<QueryAnalysisResponse> {
    return this.client.call(
      LUCIBisectionService.SERVICE,
      'QueryAnalysis',
      request
    );
  }

  async listAnalyses(
    request: ListAnalysesRequest
  ): Promise<ListAnalysesResponse> {
    return this.client.call(
      LUCIBisectionService.SERVICE,
      'ListAnalyses',
      request
    );
  }
}

export type AnalysisStatus =
  | 'ANALYSIS_STATUS_UNSPECIFIED'
  | 'CREATED'
  | 'RUNNING'
  | 'FOUND'
  | 'NOTFOUND'
  | 'ERROR'
  | 'SUSPECTFOUND';

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
  | 'RERUN_STATUS_CANCELED';

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
  verificationDetails: SuspectVerificationDetails
}

export interface NthSectionAnalysisResult {
  status: AnalysisStatus;
  startTime: string,
  endTime: string,
  suspect?: GitilesCommit;
  remainingNthSectionRange?: RegressionRange;
  reruns: SingleRerun[];
  blameList: BlameList;
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
}

export interface RerunResult {
  rerunStatus: RerunStatus;
}

export interface CL {
  commitID: string;
  title: string;
  reviewURL: string;
}

export interface RevertCL {
  cl: CL;
  status: string;
  submitTime: string;
  commitPosition: string;
}

export interface PrimeSuspect {
  cl: CL;
  culpritStatus: string;
  accuseSource: string;
}
