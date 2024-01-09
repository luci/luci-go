// Copyright 2024 The LUCI Authors.
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

export type TreesJson = TreeJson[];

export interface TreeJson {
  name: string;
  display_name: string;
  default_monorail_project_name?: string; // default to 'chromium' if undefined.
  bug_queue_label?: string;
}

export const treeJsonFromName = (treeName: string): TreeJson | null => {
  // trees-json is a string containing json, so we need one parse to get the string, and another to get the structure.
  const text = document.getElementById('trees-json')?.innerText;
  if (!text) return null;
  const trees = JSON.parse(JSON.parse(text));
  return trees.filter((t: TreeJson) => t.name == treeName)?.[0];
};

// TODO: AlertJson fields were added based on example data.  There may be missing or incorrect fields.
export interface AlertJson {
  key: string;
  title: string;
  body: string;
  severity: number;
  time: number;
  start_time: number;
  links: null;
  tags: null;
  type: string;
  extension: AlertExtensionJson;
  resolved: boolean;
}

// TODO: AlertExtensionJson fields were added based on example data.  There may be missing or incorrect fields.
export interface AlertExtensionJson {
  builders: AlertBuilderJson[];
  culprits: null;
  has_findings: boolean;
  is_finished: boolean;
  is_supported: boolean;
  reason: AlertReasonJson;
  regression_ranges: RegressionRangeJson[];
  suspected_cls: null;
  tree_closer: false;
  luci_bisection_result?: LuciBisectionResult;
}

// TODO: AlertBuilderJson fields were added based on example data.  There may be missing or incorrect fields.
export interface AlertBuilderJson {
  bucket: string;
  build_status: string;
  builder_group: string;
  count: number;
  failing_tests_trunc: string;
  first_failing_rev: RevisionJson;
  first_failure: number;
  first_failure_build_number: number;
  first_failure_url: string;
  last_passing_rev: RevisionJson;
  latest_failure: number;
  latest_failure_build_number: number;
  latest_failure_url: string;
  latest_passing: number;
  name: string;
  project: string;
  start_time: number;
  url: string;
}

// TODO: RevisionJson fields were added based on example data.  There may be missing or incorrect fields.
export interface RevisionJson {
  author: string;
  branch: string;
  commit_position: number;
  description: string;
  git_hash: string;
  host: string;
  link: string;
  repo: string;
  when: number;
}

// TODO: AlertReasonJson fields were added based on example data.  There may be missing or incorrect fields.
export interface AlertReasonJson {
  num_failing_tests: number;
  step: string;
  tests: AlertReasonTestJson[];
}

// TODO: AlertReasonTestJson fields were added based on example data.  There may be missing or incorrect fields.
export interface AlertReasonTestJson {
  test_name: string;
  test_id: string;
  realm: string;
  variant_hash: string;
  cluster_name: string;
  cur_counts: TestResultCountsJson;
  prev_counts: TestResultCountsJson;
  cur_start_hour: string;
  prev_end_hour: string;
  ref_hash: string;
  regression_end_position: number;
  regression_start_position: number;
}

export interface TestResultCountsJson {
  total_results: number;
  unexpected_results: number;
}

// TODO: RegressionRangeJson fields were added based on example data.  There may be missing or incorrect fields.
export interface RegressionRangeJson {
  host: string;
  positions: string[];
  repo: string;
  revisions: string[];
  revisions_with_results: null;
  url: string;
}

// TODO: AnnotationJson fields were added based on example data.  There may be missing or incorrect fields.
export interface AnnotationJson {
  KeyDigest: string;
  ModificationTime: string;
  Tree: string;
  bug_data: { [id: number]: AnnotationBugDataJson };
  bugs: BugId[];
  comments: null;
  group_id: string;
  key: string;
  snoozeTime: number | null;
}

export interface BugId {
  id: number;
  projectId: string;
}

export interface AnnotationBugDataJson {
  id: string;
  projectId: string;
  status: string;
  summary: string;
}

export interface BugQueueJson {
  issues: BugJson[];
  extras: {
    priority_field: string;
  };
}

export interface BugJson {
  name: string;
  summary: string;
  state: number;
  status: {
    status: string;
    derivation: number;
  };
  reporter: string;
  owner?: {
    user: string;
    derivation: number;
  };
  labels: {
    label: string;
    derivation: number;
  }[];
  attachment_count?: number;
  components?: {
    component: string;
    derivation: number;
  }[];
  cc_users?: {
    user: string;
    derivation: number;
  }[];
  field_values: {
    field: string;
    value: string;
    derivation: number;
  }[];
  create_time: {
    seconds: number;
  };
  modify_time: {
    seconds: number;
  };
  component_modify_time: {
    seconds: number;
  };
  status_modify_time: {
    seconds: number;
  };
  owner_modify_time: {
    seconds: number;
  };
  star_count: number;
}

export interface LuciBisectionResult {
  analysis?: LuciBisectionAnalysis;
  is_supported?: boolean;
  failed_bbid?: string;
}

export interface LuciBisectionAnalysis {
  analysis_id: string;
  heuristic_result?: HeuristicAnalysis;
  nth_section_result?: NthSectionAnalysis;
  culprits?: Culprit[];
}

export interface HeuristicAnalysis {
  suspects: HeuristicSuspect[];
}

export interface HeuristicSuspect {
  // TODO (nqmtuan): Also display if a verification is in progress.
  reviewUrl: string;
  justification: string;
  score: number;
  confidence_level: number;
}

export interface NthSectionAnalysis {
  suspect?: NthSectionSuspect;
  remaining_nth_section_range?: RegressionRange;
}

export interface NthSectionSuspect {
  reviewUrl: string;
  reviewTitle: string;
}

export interface RegressionRange {
  last_passed: GitilesCommit;
  first_failed: GitilesCommit;
}

export interface GitilesCommit {
  host: string;
  project: string;
  ref: string;
  id: string;
}

export interface Culprit {
  review_url: string;
  review_title: string;
}

export interface Bug {
  number: string;
  link: string;
  summary: string | undefined;
  priority: number | undefined;
  status: string | undefined;
  labels: string[];
}

export const bugFromJson = (bug: BugJson, priorityField: string): Bug => {
  // TODO: Make this work for other projects and buganizer.
  const queueLabel = 'Sheriff-Chromium';
  const number = /[0-9]+/.exec(bug.name)?.[0] || '';
  return {
    number: number,
    link: `https://crbug.com/${number}`,
    summary: bug.summary,
    priority: parseInt(
      bug.field_values.filter((f) => f.field == priorityField)?.[0]?.value,
    ),
    status: bug.status.status,
    labels: bug.labels.map((l) => l.label).filter((l) => l != queueLabel),
  };
};

export const bugFromId = (bug: string): Bug => {
  // TODO: Make this work for other projects and buganizer.
  return {
    number: bug,
    labels: [],
    link: `https://crbug.com/${bug}`,
    priority: undefined,
    status: undefined,
    summary: undefined,
  };
};
