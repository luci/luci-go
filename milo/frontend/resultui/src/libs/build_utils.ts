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

import { router } from '../routes';
import { Build, BuilderID, BuildInfraSwarming, BuildStatus, GerritChange, GitilesCommit } from '../services/buildbucket';
import { Link, StepExt } from '../services/build_page';

export function getURLForBuild(build: Build): string {
  return router.urlForName(
      'build',
      {
      'project': build.builder.project,
      'bucket': build.builder.bucket,
      'builder': build.builder.builder,
      'build_num_or_id': 'b' + build.id,
      },
  );
}

export function getURLForBuilder(builder: BuilderID): string {
  return `/p/${builder.project}/builders/${builder.bucket}/${builder.builder}`;
}

export function getURLForProject(proj: string): string {
  return `/p/${proj}`;
}

export function getLegacyURLForBuild(builder: BuilderID , buildNumOrId: string) {
  return `${getURLForBuilder(builder)}/${buildNumOrId}`;
}

export function getDisplayNameForStatus(s: BuildStatus): string {
  const statusMap = new Map([
    [BuildStatus.Scheduled, 'Scheduled'],
    [BuildStatus.Started, 'Running'],
    [BuildStatus.Success, 'Success'],
    [BuildStatus.Failure, 'Failure'],
    [BuildStatus.InfraFailure, 'Infra Failure'],
    [BuildStatus.Canceled, 'Canceled'],
  ]);
  return statusMap.get(s) || 'Unknown';
}

export function getURLForGitilesCommit(commit: GitilesCommit): string {
  return `https://${commit.host}/${commit.project}/+/${commit.id}`;
}

export function getURLForGerritChange(change: GerritChange): string {
  return `https://${change.host}/c/${change.change}/${change.patchset}`;
}

export function getURLForSwarmingTask(swarming: BuildInfraSwarming): string {
  return `https://${swarming.hostname}/task?id=${swarming.task_id}&o=true&w=true`;
}

// getBotLink generates a link to a swarming bot.
export function getBotLink(swarming: BuildInfraSwarming): Link | null {
  for (const dim of swarming.bot_dimensions || []) {
    if (dim.key === 'id') {
      return {
        label: dim.value,
        url: `https://${swarming.hostname}/bot?id=${dim.value}`,
        aria_label: `swarming bot ${dim.value}`,
      };
    }
  }
  return null;
}

// getLogdogRawUrl generates raw link from a logdog:// url
export function getLogdogRawUrl(logdogURL: string): string | null {
  const match = /^(logdog:\/\/)([^\/]*)\/(.+)$/.exec(logdogURL);
  if (!match) {
    return null;
  }
  return `https://${match[2]}/logs/${match[3]}?format=raw`;
}

// stepSucceededRecursive returns true if a step and its descendants succeeded.
// UI wise, we should expand those steps by default.
export function stepSucceededRecursive(step: StepExt):boolean {
  if (step.status !== BuildStatus.Success) {
    return false;
  }
  if (!step.children) {
    return true;
  }
  return step.children.map(s => stepSucceededRecursive(s)).reduce((a, s) => a && s, true);
}
