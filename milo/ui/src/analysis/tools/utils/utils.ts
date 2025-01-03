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

import { ClusterId } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';

/**
 * Construct a link to a luci-analysis cluster.
 */
export function makeClusterLink(project: string, clusterId: ClusterId) {
  return `https://${SETTINGS.luciAnalysis.uiHost || SETTINGS.luciAnalysis.host}/p/${project}/clusters/${clusterId.algorithm}/${clusterId.id}`;
}

/**
 * Construct a link to a luci-analysis rule page.
 */
export function makeRuleLink(project: string, ruleId: string) {
  return `https://${SETTINGS.luciAnalysis.uiHost || SETTINGS.luciAnalysis.host}/p/${project}/rules/${ruleId}`;
}
