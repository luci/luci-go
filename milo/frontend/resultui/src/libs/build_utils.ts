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

import { BuildExt } from '../models/build_ext';
import { Link } from '../models/link';
import { router } from '../routes';
import { Build, BuilderID, BuildInfraSwarming, GerritChange, GitilesCommit } from '../services/buildbucket';

export function getURLPathForBuild(build: Pick<Build, 'builder' | 'number' | 'id'>): string {
  return router.urlForName('build', {
    project: build.builder.project,
    bucket: build.builder.bucket,
    builder: build.builder.builder,
    build_num_or_id: build.number ? build.number.toString() : `b${build.id}`,
  });
}

export function getURLPathForBuilder(builder: BuilderID): string {
  return (
    `${getURLPathForProject(builder.project)}/builders/${encodeURIComponent(builder.bucket)}` +
    `/${encodeURIComponent(builder.builder)}`
  );
}

export function getURLPathForProject(proj: string): string {
  return `/p/${encodeURIComponent(proj)}`;
}

export function getLegacyURLPathForBuild(builder: BuilderID, buildNumOrId: string) {
  return `/old${getURLPathForBuilder(builder)}/${buildNumOrId}`;
}

export function getGitilesRepoURL(commit: Pick<GitilesCommit, 'host' | 'project'>) {
  return `https://${commit.host}/${commit.project}`;
}

export function getURLForGitilesCommit(commit: GitilesCommit): string {
  return `${getGitilesRepoURL(commit)}/+/${commit.id}`;
}

export function getURLForGerritChange(change: GerritChange): string {
  return `https://${change.host}/c/${change.change}/${change.patchset}`;
}

export function getURLForSwarmingTask(hostname: string, taskId: string): string {
  return `https://${hostname}/task?id=${taskId}&o=true&w=true`;
}

// getBotLink generates a link to a swarming bot.
export function getBotLink(swarming: BuildInfraSwarming): Link | null {
  for (const dim of swarming.botDimensions || []) {
    if (dim.key === 'id') {
      return {
        label: dim.value,
        url: `https://${swarming.hostname}/bot?id=${dim.value}`,
        ariaLabel: `swarming bot ${dim.value}`,
      };
    }
  }
  return null;
}

// getBuildbucketLink generates a link to a buildbucket RPC explorer page for
// the given build.
export function getBuildbucketLink(buildbucketHost: string, buildId: string): Link {
  return {
    label: buildId,
    url: `https://${buildbucketHost}/rpcexplorer/services/buildbucket.v2.Builds/GetBuild?${new URLSearchParams([
      [
        'request',
        JSON.stringify({
          id: buildId,
        }),
      ],
    ]).toString()}`,
    ariaLabel: 'Buildbucket RPC explorer for build',
  };
}

// getLogdogRawUrl generates raw link from a logdog:// url
export function getLogdogRawUrl(logdogURL: string): string | null {
  const match = /^(logdog:\/\/)([^/]*)\/(.+)$/.exec(logdogURL);
  if (!match) {
    return null;
  }
  return `https://${match[2]}/logs/${match[3]}?format=raw`;
}

export function getSafeUrlFromBuildset(buildset: string): string | null {
  {
    const match = buildset.match(/^patch\/gerrit\/([\w-]+\.googlesource\.com)\/(\d+\/\d+)$/);
    if (match) {
      const [, host, cl] = match as string[];
      return `https://${host}/c/${cl}`;
    }
  }
  {
    const match = buildset.match(/^commit\/gitiles\/([\w-]+\.googlesource\.com\/.+)$/);
    if (match) {
      const [, url] = match as string[];
      return `https://${url}`;
    }
  }
  return null;
}

/**
 * Renders build bug template.
 *
 * The following patterns will be replaced with the actual values:
 *  * {{.Build.Builder.Project}}
 *  * {{.Build.Builder.Bucket}}
 *  * {{.Build.Builder.Builder}}
 *  * {{.MiloBuildUrl}}
 *  * {{.MiloBuilderUrl}}
 */
// TODO(weiweilin): support more go template syntax if needed.
export function renderBuildBugTemplate(template: string, build: BuildExt): string {
  return template
    .replace('{{.Build.Builder.Project}}', build.builder.project)
    .replace('{{.Build.Builder.Bucket}}', build.builder.bucket)
    .replace('{{.Build.Builder.Builder}}', build.builder.builder)
    .replace('{{.MiloBuildUrl}}', window.location.origin + getURLPathForBuild(build))
    .replace('{{.MiloBuilderUrl}}', window.location.origin + getURLPathForBuilder(build.builder));
}
