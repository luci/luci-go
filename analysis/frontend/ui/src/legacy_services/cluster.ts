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
  Changelist,
  ClusterId,
  MetricId,
  Variant,
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

  async get(request: GetClusterRequest): Promise<Cluster> {
    return this.client.call(ClustersService.SERVICE, 'Get', request);
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

  async queryExoneratedTestVariants(request: QueryClusterExoneratedTestVariantsRequest): Promise<QueryClusterExoneratedTestVariantsResponse> {
    return this.client.call(ClustersService.SERVICE, 'QueryExoneratedTestVariants', request);
  }
  async queryHistory(request: QueryClusterHistoryRequest): Promise<QueryClusterHistoryResponse> {
    return this.client.call(ClustersService.SERVICE, 'QueryHistory', request);
  }
}

export interface GetClusterRequest {
  // The resource name of the cluster to retrieve.
  // Format: projects/{project}/clusters/{cluster_algorithm}/{cluster_id}.
  name: string;
}

export type ClusterMetrics = { [metricId: string]: TimewiseCounts | undefined; };

export interface Cluster {
  // The resource name of the cluster.
  // Format: projects/{project}/clusters/{cluster_algorithm}/{cluster_id}.
  name: string;
  // Whether there is a recent example in the cluster.
  hasExample?: boolean;
  // A human-readable name for the cluster.
  // Only populated for suggested clusters where has_example = true.
  title?: string;
  // The values of metrics associated with the cluster. The map key is the ID
  // of the metric (e.g. "human-cls-failed-presubmit").
  metrics?: ClusterMetrics;
  // The failure association rule equivalent to the cluster. Populated only
  // for suggested clusters where has_example = true; for rule-based
  // clusters, lookup the rule instead. Used to facilitate creating a new
  // rule based on this cluster.
  equivalentFailureAssociationRule: string | undefined;
}

export interface TimewiseCounts {
  // The metric value for the last day.
  oneDay: Counts;
  // The metric value for the last three days.
  threeDay: Counts;
  // The metric value for the last week.
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

export interface TimeRange {
  // RFC 3339 encoded date/time which is the start of the range, inclusive.
  // e.g. "2019-01-25T02:00:00.000Z"
  earliest: string;

  // RFC 3339 encoded date/time which is the end of the range, exclusive.
  // e.g. "2019-01-26T02:00:00.000Z"
  latest: string;
}

export type ClusterSummaryView =
  | 'CLUSTER_SUMMARY_VIEW_UNSPECIFIED'
  | 'BASIC'
  | 'FULL';

export interface QueryClusterSummariesRequest {
  // The LUCI project.
  project: string;

  // An AIP-160-style filter on the failures that are used as input to
  // clustering.
  failureFilter: string;

  // An AIP-132-style order_by clause, which specifies the sort order
  // of the result.
  orderBy: string;

  // The time range over which to get the cluster summaries.
  timeRange: TimeRange;

  // The resource name(s) of the metrics to include for each cluster
  // in the response.
  metrics?: string[];

  // The level of detail that the returned cluster summaries should have.
  view?: ClusterSummaryView;
}

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
  // The values of cluster metrics. The key of the map is the identifier
  // of the metric (e.g. "human-cls-failed-presubmit").
  metrics?: { [metricID: MetricId]: MetricValue };
}

export interface MetricValue {
  // 64-bit integer serialized as a string.
  value?: string;
  // Metric values for each 24-hour period within the queried time range,
  // in reverse chronological order, as 64-bit integers serialized as strings.
  dailyBreakdown?: string[];
}

export interface QueryClusterFailuresRequest {
  // The resource name of the cluster failures to retrieve.
  // Format: projects/{project}/clusters/{cluster_algorithm}/{cluster_id}/failures.
  parent: string;

  // The resource name(s) of the metric for which failures should
  // be displayed.
  // Format: projects/{project}/metrics/{metric_id}.
  //
  // If no metric is specified here, then no filtering is performed
  // and all failures can be returned.
  metricFilter?: string;
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
  | 'QUICK_DRY_RUN'
  | 'NEW_PATCHSET_RUN';

// Refer to luci.analysis.v1.PresubmitRunStatus for documentation.
export type PresubmitRunStatus =
  'PRESUBMIT_RUN_STATUS_UNSPECIFIED'
  | 'PRESUBMIT_RUN_STATUS_SUCCEEDED'
  | 'PRESUBMIT_RUN_STATUS_FAILED'
  | 'PRESUBMIT_RUN_STATUS_CANCELED';

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
  // The status of the presubmit run.
  status: PresubmitRunStatus;
}

// Refer to luci.analysis.v1.DistinctClusterFailure for documentation.
export interface DistinctClusterFailure {
  // The identity of the test.
  testId: string;

  // The variant. Describes a way of running a test.
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
  count: number;
}

export interface QueryClusterExoneratedTestVariantsRequest {
  // The resource name of the cluster exonerated test variants to retrieve.
  // Format: projects/{project}/clusters/{cluster_algorithm}/{cluster_id}/exoneratedTestVariants.
  parent: string;
}

export interface QueryClusterExoneratedTestVariantsResponse {
  // A list of test variants in the cluster which have exonerated critical
  // failures. Ordered by recency of the exoneration (most recent exonerations
  // first) and limited to at most 100 test variants.
  testVariants?: ClusterExoneratedTestVariant[];
}

// ClusterExoneratedTestVariant represents a test variant in a cluster
// which has been exonerated. A cluster test variant is the subset
// of a test variant that intersects with the failures of a cluster.
export interface ClusterExoneratedTestVariant {
  // A unique identifier of the test in a LUCI project.
  testId: string;

  // The variant. Describes a way of running a test.
  variant?: Variant;

  // The number of critical (presubmit-blocking) failures in the
  // cluster which have been exonerated on this test variant
  // in the last week.
  criticalFailuresExonerated: number;

  // The partition time of the most recent exoneration of a
  // critical failure.
  // RFC 3339 encoded date/time.
  lastExoneration: string;
}

export interface QueryClusterHistoryRequest {
  project: string;
  failureFilter: string;
  days: number;
  metrics: string[];
}

export interface QueryClusterHistoryResponse {
  days: ClusterHistoryDay[];
}

export interface ClusterHistoryDay {
  date: string;
  metrics: { [metricId: string]: number };
}
