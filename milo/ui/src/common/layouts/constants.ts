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

import { UiPage } from '@/common/constants/view';

export const DRAWER_WIDTH = 250;

export const PAGE_LABEL_MAP = Object.freeze({
  [UiPage.BuilderSearch]: 'Builder search',
  [UiPage.ProjectSearch]: 'Projects',
  [UiPage.Builders]: 'Builders',
  [UiPage.Scheduler]: 'Scheduler',
  [UiPage.Bisection]: 'Bisection',
  [UiPage.TestHistory]: 'Test history',
  [UiPage.FailureClusters]: 'Failure clusters',
  [UiPage.Testhaus]: 'Testhaus',
  [UiPage.Crosbolt]: 'Crosbolt',
  [UiPage.BuilderGroups]: 'Builder groups',
  [UiPage.SoM]: 'Sheriff-o-Matic',
  [UiPage.CQStatus]: 'CQ Status Dashboard',
  [UiPage.Goldeneye]: 'Goldeneye',
  [UiPage.ChromiumDash]: 'ChromiumDash',
  [UiPage.ReleaseNotes]: "What's new",
  [UiPage.TestVerdict]: 'Test verdict',
  [UiPage.TreeStatus]: 'Tree status',
  [UiPage.Monitoring]: 'Monitoring',
  [UiPage.RecentRegressions]: 'Recent regressions',
  [UiPage.RegressionDetails]: 'Regression details',
  [UiPage.LogSearch]: 'Log search',
  [UiPage.Blamelist]: 'Blamelist',
  [UiPage.AuthServiceGroups]: 'Groups',
  [UiPage.AuthServiceLookup]: 'User lookup',
  [UiPage.Clusters]: 'Clusters',
  [UiPage.Rules]: 'Rules',
});
