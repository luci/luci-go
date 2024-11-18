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

/*
 * An enum representing the pages
 */
export enum UiPage {
  BuilderSearch = 'builder-search',
  ProjectSearch = 'project-search',
  Builders = 'builders',
  Scheduler = 'scheduler',
  Bisection = 'bisection',
  TestHistory = 'test-history',
  FailureClusters = 'failure-clusters',
  Testhaus = 'test-haus',
  Crosbolt = 'crosbolt',
  BuilderGroups = 'builder-groups',
  SoM = 'som',
  CQStatus = 'cq-status',
  Goldeneye = 'goldeneye',
  ChromiumDash = 'chromium-dash',
  ReleaseNotes = 'release-notes',
  TestVerdict = 'test-verdict',
  TreeStatus = 'tree-status',
  Monitoring = 'monitoring',
  RecentRegressions = 'recent-regressions',
  RegressionDetails = 'regression-details',
  LogSearch = 'log-search',
  Blamelist = 'blamelist',
  AuthService = 'authdb-group',
}

export enum CommonColors {
  FADED_TEXT = '#888',
}
