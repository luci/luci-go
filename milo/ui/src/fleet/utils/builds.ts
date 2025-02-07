// Copyright 2025 The LUCI Authors.
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

// Check if we're using the dev version of build bucket to adjust our RPCs to
// go through properly.
// TODO: Find out if there's a better pattern for this besides hardcoding
export const USING_BUILD_BUCKET_DEV =
  SETTINGS.buildbucket.host.includes('cr-buildbucket-dev');

const MILO_PROD = 'ci.chromium.org';
const SWARMING_CHROMEOS_PROD = 'chromeos-swarming.appspot.com';

// TODO: b/394901923 - Because there is no ChromeOS Swarming dev, right now,
// all task history data is bring pulled from ChromeOS Swarming prod. This
// means we need additional hardcoding to make sure dev testing produces output
// that feels meaningful.
export const DEVICE_TASKS_MILO_HOST = MILO_PROD;
export const DEVICE_TASKS_SWARMING_HOST = SWARMING_CHROMEOS_PROD;

// Note that this is different from SETTINGS.milo.host or the host of the
// current server because we are looking for the frontend associated with
// our current SETTINGS.buildbucket.host config.
export const FLEET_BUILDS_MILO_HOST = USING_BUILD_BUCKET_DEV
  ? 'luci-milo-dev.appspot.com'
  : MILO_PROD;

/**
 * When we're in dev, use a dev Swarming because we're not scheduling
 * builds in prod. Note: There is no ChromeOS Swarming dev project.
 */
// TODO: b/394901923 - Find a way to make it possible to view dev BuildBucket
// tasks in Swarming task list. Right now, because dev Swarming is used for
// scheduling tasks but prod Swarming is used for viewing task history, it's
// not possible to see the task you just scheduled in dev.
export const FLEET_BUILDS_SWARMING_HOST = USING_BUILD_BUCKET_DEV
  ? 'chromium-swarm-dev.appspot.com'
  : SWARMING_CHROMEOS_PROD;

export interface BuildIdentifier {
  project?: string;
  bucket?: string;
  builder?: string;
  buildId?: string;
}

export function generateBuildUrl(
  { project, bucket, builder, buildId }: BuildIdentifier,
  miloHost: string = FLEET_BUILDS_MILO_HOST,
) {
  return `https://${miloHost}/p/${project}/builders/${bucket}/${builder}/b${buildId}`;
}

/**
 * @param tags A list of tags in the format used by the Swarming V2 API.
 *   ie: ["authenticated:project:chromeos", "buildbucket_bucket:chromeos/labpack_runner"]
 * @returns a URL to Milo showing a build.
 */
export function extractBuildUrlFromTagData(
  tags: readonly string[],
  miloHost: string = FLEET_BUILDS_MILO_HOST,
): string | undefined {
  const tagMap = tagsToMap([...tags]);
  const buildId = tagMap.get('buildbucket_build_id');
  const scopedBucket = tagMap.get('buildbucket_bucket');
  const builder = tagMap.get('builder');
  if (!buildId || !scopedBucket || !builder) return undefined;

  // Swarming returns Buildbucket data in a format like
  // `chromeos/labpack_runner`.
  if (!scopedBucket.includes('/')) return undefined;
  const [project, bucket] = scopedBucket.split('/');

  return generateBuildUrl({ project, builder, buildId, bucket }, miloHost);
}

/**
 * Turn a list of tags in the format given by the Swarming Task API into
 * a Map.
 *
 * @param tags Array of key value pairs in the format `key:value`
 * @returns Map indexed by tag key where the value is the value.
 */
export function tagsToMap(tags: readonly string[]): Map<string, string> {
  const keyValues =
    tags.map((tag) => {
      if (!tag || !tag.includes(':')) {
        throw new Error('Unexpected tag format.');
      }
      const [key, ...values] = tag.split(':');

      return [key, values.join(':')] as [string, string];
    }) || [];
  return new Map(keyValues);
}
