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

import Mustache from 'mustache';

import { Link } from '@/common/models/link';
import { logging } from '@/common/tools/logging';
import {
  getBuilderURLPath,
  getInvURLPath,
  getSwarmingTaskURL,
} from '@/common/tools/url_utils';
import {
  Build,
  BuildInfra_Swarming,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/build.pb';
import { getBotUrl } from '@/swarming/tools/utils';

// getBotLink generates a link to a swarming bot.
export function getBotLink(
  swarming: Pick<BuildInfra_Swarming, 'botDimensions' | 'hostname'>,
): Link | null {
  for (const dim of swarming.botDimensions) {
    if (dim.key === 'id') {
      return {
        label: dim.value,
        url: getBotUrl(swarming.hostname, dim.value),
        ariaLabel: `swarming bot ${dim.value}`,
      };
    }
  }
  return null;
}

// getBotLink generates a link to a swarming bot.
export function getInvocationLink(invocationName: string): Link | null {
  const invId = invocationName.slice('invocations/'.length);
  return {
    label: invocationName,
    url: getInvURLPath(invId),
    ariaLabel: `result db invocation ${invocationName}`,
  };
}

// getBuildbucketLink generates a link to a buildbucket RPC explorer page for
// the given build.
export function getBuildbucketLink(
  buildbucketHost: string,
  buildId: string,
): Link {
  return {
    label: buildId,
    url: `https://${buildbucketHost}/rpcexplorer/services/buildbucket.v2.Builds/GetBuild?${new URLSearchParams(
      [
        [
          'request',
          JSON.stringify({
            id: buildId,
          }),
        ],
      ],
    ).toString()}`,
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

export function getSafeUrlFromTagValue(tagValue: string): string | null {
  {
    const match = tagValue.match(
      /^patch\/gerrit\/([\w-]+\.googlesource\.com)\/(\d+\/\d+)$/,
    );
    if (match) {
      const [, host, cl] = match as string[];
      return `https://${host}/c/${cl}`;
    }
  }
  {
    const match = tagValue.match(
      /^commit\/gitiles\/([\w-]+\.googlesource\.com\/.+)$/,
    );
    if (match) {
      const [, url] = match as string[];
      return `https://${url}`;
    }
  }
  {
    const match = tagValue.match(
      /^build\/milo\/([\w\-_ ]+)\/([\w\-_ ]+)\/([\w\-_ ]+)\/(\d+)$/,
    );
    if (match) {
      const [, project, bucket, builder, buildIdOrNum] = match as string[];
      return `${getBuilderURLPath({
        project,
        bucket,
        builder,
      })}/${buildIdOrNum}`;
    }
  }
  {
    const match = tagValue.match(
      /^task\/swarming\/(chrome-swarming|chromium-swarm)\/(.+)$/,
    );
    if (match) {
      const [, instance, taskId] = match as string[];
      return getSwarmingTaskURL(`${instance}.appspot.com`, taskId);
    }
  }
  return null;
}

const RE_BUG_URL =
  /https:\/\/(bugs\.chromium\.org|b\.corp\.google\.com)(\/*.)?/;

/**
 * Renders Project.BugUrlTemplate. See the definition for Project.BugUrlTemplate
 * https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/main/milo/api/config/project.proto#70
 * for details.
 */
export function renderBugUrlTemplate(
  urlTemplate: string,
  build: DeepNonNullable<Pick<Build, 'id' | 'builder'>>,
  miloOrigin = window.location.origin,
) {
  let bugUrl = '';
  try {
    bugUrl = Mustache.render(urlTemplate, {
      build: {
        builder: {
          project: encodeURIComponent(build.builder.project),
          bucket: encodeURIComponent(build.builder.bucket),
          builder: encodeURIComponent(build.builder.builder),
        },
      },
      milo_build_url: encodeURIComponent(miloOrigin + `/b/${build.id}`),
      milo_builder_url: encodeURIComponent(
        miloOrigin + getBuilderURLPath(build.builder),
      ),
    });
  } catch (_e) {
    logging.warn(
      'failed to render the bug URL template. Please ensure the bug URL template is a valid mustache template.',
    );
    // Do nothing.
  }
  if (!RE_BUG_URL.test(bugUrl)) {
    // IDEA: instead of failing silently, we could link users to a page that
    // shows the error log and how to fix it.
    logging.warn('the bug URL has an invalid/disallowed domain name or scheme');
    bugUrl = '';
  }
  return bugUrl;
}

// getCipdLink generates a link to chrome-infra-package webpage for the given
// pkgName and version.
export function getCipdLink(pkgName: string, version: string): Link {
  return {
    label: version,
    url: `https://chrome-infra-packages.appspot.com/p/${pkgName}/+/${version}`,
    ariaLabel: `cipd url for ${pkgName}`,
  };
}

const LEGACY_BUCKET_ID_RE = /^luci\.([^.]+)\.(.+)$/;

/**
 * Parses a legacy bucket ID (e.g. `luci.${project}.${bucket}`).
 */
// TODO(weiweilin): find all usage of the legacy bucket ID and convert them
// to the new ID format.
export function parseLegacyBucketId(bucketId: string) {
  const match = bucketId.match(LEGACY_BUCKET_ID_RE);
  if (!match) {
    return null;
  }

  return { project: match[1], bucket: match[2] };
}
