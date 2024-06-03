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

import { Link } from '@mui/material';

import { makeRuleLink } from '@/analysis/tools/utils';
import { OutputClusterEntry } from '@/analysis/types';
import { AssociatedBug } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';

interface BugClusterEntry extends OutputClusterEntry {
  readonly bug: AssociatedBug;
}

export interface AssociatedBugsBadgeTooltipProps {
  readonly project: string;
  readonly clusters: readonly OutputClusterEntry[];
}

export function AssociatedBugsBadgeTooltip({
  project,
  clusters,
}: AssociatedBugsBadgeTooltipProps) {
  const bugClusters = clusters.filter((c) => c.bug) as BugClusterEntry[];
  return (
    <table>
      <thead>
        <tr>
          <td colSpan={2}>
            This failure is associated with the following bug(s):
          </td>
        </tr>
      </thead>
      <tbody>
        {bugClusters.map((c) => (
          <tr key={c.clusterId.id}>
            <td>
              <Link href={c.bug.url}>{c.bug.linkText}</Link>
            </td>
            <td width="1px">
              <Link
                href={makeRuleLink(project, c.clusterId.id)}
                target="_blank"
                rel="noopener"
              >
                Failures
              </Link>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
