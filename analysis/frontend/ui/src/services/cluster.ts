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
  AssociatedBug,
  ClusterId,
} from './shared_models';

export const getClustersService = () => {
  const client = new AuthorizedPrpcClient();
  return new ClustersService(client);
};

// A service to handle cluster-related gRPC requests.
export class ClustersService {
  private static SERVICE = 'luci.analysis.v1.Clusters';

  client: AuthorizedPrpcClient;

  constructor(client: AuthorizedPrpcClient) {
    this.client = client;
  }

  async batchGet(request: BatchGetClustersRequest): Promise<BatchGetClustersResponse> {
    return this.client.call(ClustersService.SERVICE, 'BatchGet', request);
  }

  async getReclusteringProgress(request: GetReclusteringProgressRequest): Promise<ReclusteringProgress> {
    return this.client.call(ClustersService.SERVICE, 'GetReclusteringProgress', request);
  }

  async queryClusterSummaries(request: QueryClusterSummariesRequest): Promise<QueryClusterSummariesResponse> {
    return this.client.call(ClustersService.SERVICE, 'QueryClusterSummaries', request);
  }

  async queryClusterFailures(request: QueryClusterFailuresRequest): Promise<QueryClusterFailuresResponse> {
    return this.client.call(ClustersService.SERVICE, 'QueryClusterFailures', request);
  }
}

export interface BatchGetClustersRequest {
  // The LUCI project shared by all clusters to retrieve.
  // Required.
  // Format: projects/{project}.
  parent: string;

  // The resource name of the clusters retrieve.
  // Format: projects/{project}/clusters/{cluster_algorithm}/{cluster_id}.
  // At most 1,000 clusters may be requested at a time.
  names: string[];
}

export interface BatchGetClustersResponse {
  clusters?: Cluster[];
}

export interface Cluster {
  // The resource name of the cluster.
  // Format: projects/{project}/clusters/{cluster_algorithm}/{cluster_id}.
  name: string;
  // Whether there is a recent example in the cluster.
  hasExample?: boolean;
  // A human-readable name for the cluster.
  // Only populated for suggested clusters where has_example = true.
  title?: string;
  // The total number of user changelists which failed presubmit.
  userClsFailedPresubmit: MetricValues;
  // The total number of failures in the cluster that occurred on tryjobs
  // that were critical (presubmit-blocking) and were exonerated for a
  // reason other than NOT_CRITICAL or UNEXPECTED_PASS.
  criticalFailuresExonerated: MetricValues;
  // The total number of failures in the cluster.
  failures: MetricValues;
  // The failure association rule equivalent to the cluster. Populated only
  // for suggested clusters where has_example = true; for rule-based
  // clusters, lookup the rule instead. Used to facilitate creating a new
  // rule based on this cluster.
  equivalentFailureAssociationRule: string | undefined;
}

export interface MetricValues {
  // The impact for the last day.
  oneDay: Counts;
  // The impact for the last three days.
  threeDay: Counts;
  // The impact for the last week.
  sevenDay: Counts;
}

export interface Counts {
  // The value of the metric (summed over all failures).
  // 64-bit integer serialized as a string.
  nominal?: string;
}

export interface GetReclusteringProgressRequest {
  // The name of the reclustering progress resource.
  // Format: projects/{project}/reclusteringProgress.
  name: string;
}

// ReclusteringProgress captures the progress re-clustering a
// given LUCI project's test results with a specific rules
// version and/or algorithms version.
export interface ReclusteringProgress {
  // ProgressPerMille is the progress of the current re-clustering run,
  // measured in thousandths (per mille).
  progressPerMille?: number;
  // Last is the goal of the last completed re-clustering run.
  last: ClusteringVersion;
  // Next is the goal of the current re-clustering run. (For which
  // ProgressPerMille is specified.)
  // It may be the same as the goal of the last completed reclustering run.
  next: ClusteringVersion;
}

// ClusteringVersion captures the rules and algorithms a re-clustering run
// is re-clustering to.
export interface ClusteringVersion {
  rulesVersion: string; // RFC 3339 encoded date/time.
  configVersion: string; // RFC 3339 encoded date/time.
  algorithmsVersion: number;
}

export interface QueryClusterSummariesRequest {
  // The LUCI project.
  project: string;

  // An AIP-160 style filter on the failures that are used as input to
  // clustering.
  failureFilter: string;

  // An AIP-132 style order_by clause, which specifies the sort order
  // of the result.
  orderBy: string;
}

export type SortableMetricName = 'presubmit_rejects' | 'critical_failures_exonerated' | 'failures';

export interface QueryClusterSummariesResponse {
  clusterSummaries?: ClusterSummary[];
}

export interface ClusterSummary {
  // The identity of the cluster.
  clusterId: ClusterId;
  // A one-line description of the cluster.
  title: string;
  // The bug associated with the cluster. This is only present for
  // clusters defined by failure association rules.
  bug?: AssociatedBug;
  // The number of distinct user CLs rejected by the cluster.
  // 64-bit integer serialized as a string.
  presubmitRejects?: string;
  // The number of failures that were critical (on builders critical
  // to CQ succeeding and not exonerated for non-criticality)
  // and exonerated.
  // 64-bit integer serialized as a string.
  criticalFailuresExonerated?: string;
  // The total number of test results in the cluster.
  // 64-bit integer serialized as a string.
  failures?: string;
}

export interface QueryClusterFailuresRequest {
  // The resource name of the cluster to retrieve failures for.
  // Format: projects/{project}/clusters/{cluster_algorithm}/{cluster_id}/failures.
  parent: string;
}

export interface QueryClusterFailuresResponse {
  // Example failures in the cluster. Limited to 2000 rows.
  failures?: DistinctClusterFailure[];
}

// The reason a test result was exonerated.
export type ExonerationReason =
    // The exoneration reason is not known to LUCI Analysis.
    'EXONERATION_REASON_UNSPECIFIED'
    // Similar unexpected results were observed in presubmit run(s) for other,
    // unrelated CL(s). (This is suggestive of the issue being present
    // on mainline but is not confirmed as there are possible confounding
    // factors, like how tests are run on CLs vs how tests are run on
    // mainline branches.)
    // Applies to unexpected results in presubmit/CQ runs only.
    | 'OCCURS_ON_OTHER_CLS'
    // Similar unexpected results were observed on a mainline branch
    // (i.e. against a build without unsubmitted changes applied).
    // (For avoidance of doubt, this includes both flakily and
    // deterministically occurring unexpected results.)
    // Applies to unexpected results in presubmit/CQ runs only.
    | 'OCCURS_ON_MAINLINE'
    // The tests are not critical to the test subject (e.g. CL) passing.
    // This could be because more data is being collected to determine if
    // the tests are stable enough to be made critical (as is often the
    // case for experimental test suites).
    | 'NOT_CRITICAL'
    // The test variant was exonerated because it contained an unexpected
    // pass.
    | 'UNEXPECTED_PASS';

// Refer to luci.analysis.v1.PresubmitRunMode for documentation.
export type PresubmitRunMode =
    'PRESUBMIT_RUN_MODE_UNSPECIFIED'
    | 'DRY_RUN'
    | 'FULL_RUN'
    | 'QUICK_DRY_RUN';

// Refer to luci.analysis.v1.BuildStatus for documentation.
export type BuildStatus =
    'BUILD_STATUS_UNSPECIFIED'
    | 'BUILD_STATUS_SUCCESS'
    | 'BUILD_STATUS_FAILURE'
    | 'BUILD_STATUS_INFRA_FAILURE'
    | 'BUILD_STATUS_CANCELED';

// Refer to luci.analysis.v1.ClusterFailureGroup.Exoneration for documentation.
export interface Exoneration {
  // The machine-readable reason for the exoneration.
  reason: ExonerationReason;
}

// Key/Value Variant pair that describes (part of) a way to run a test.
export interface VariantPair {
  key?: string;
  value?: string;
}

export interface VariantDef {
  [key: string]: string | undefined;
}

export interface Variant {
  def: VariantDef;
}

// Identity of a presubmit run.
// Refer to luci.analysis.v1.PresubmitRunId for documentation.
export interface PresubmitRunId {
  system?: string;
  id?: string;
}

// Refer to luci.analysis.v1.ClusterFailureGroup.PresubmitRun for documentation.
export interface PresubmitRun {
  // Identity of the presubmit run that contains this test result.
  presubmitRunId: PresubmitRunId;
  // The owner of the presubmit run (if any).
  owner: string;
  // The mode of the presubmit run.
  mode: PresubmitRunMode;
}

export interface Changelist {
  // Gerrit hostname, e.g. "chromium-review.googlesource.com".
  host: string;

  // Change number, encoded as a string, e.g. "12345".
  change: string;

  // Patchset number, e.g. 1.
  patchset: number;
}

// Refer to luci.analysis.v1.DistinctClusterFailure for documentation.
export interface DistinctClusterFailure {
  // The identity of the test.
  testId: string;

  // The test variant. Describes a way of running a test.
  variant?: Variant;

  partitionTime: string; // RFC 3339 encoded date/time.

  presubmitRun?: PresubmitRun;

  // Whether the build was critical to a presubmit run succeeding.
  // If the build was not part of a presubmit run, this field should
  // be ignored.
  isBuildCritical?: boolean;

  // The exonerations applied to the test variant verdict.
  exonerations?: Exoneration[];

  // The status of the build that contained this test result. Can be used
  // to filter incomplete results (e.g. where build was cancelled or had
  // an infra failure). Can also be used to filter builds with incomplete
  // exonerations (e.g. build succeeded but some tests not exonerated).
  // This is the build corresponding to ingested_invocation_id.
  buildStatus: BuildStatus;

  // The invocation from which this test result was ingested. This is
  // the top-level invocation that was ingested, an "invocation" being
  // a container of test results as identified by the source test result
  // system.
  //
  // For ResultDB, LUCI Analysis ingests invocations corresponding to
  // buildbucket builds.
  ingestedInvocationId: string;

  // Is the ingested invocation blocked by this test variant? This is
  // only true if all (non-skipped) test results for this test variant
  // (in the ingested invocation) are unexpected failures.
  //
  // Exoneration does not factor into this value; check exonerations
  // to see if the impact of this ingested invocation being blocked was
  // mitigated by exoneration.
  isIngestedInvocationBlocked?: boolean;

  changelists?: Changelist[];

  // The number of test results in the group.
  count : number;
}
