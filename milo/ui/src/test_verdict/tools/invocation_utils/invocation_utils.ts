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

const ALLOWED_SWARMING_HOSTS = [
  'chromium-swarm-dev.appspot.com',
  'chromium-swarm.appspot.com',
  'chrome-swarming.appspot.com',
];

export type ParsedInvId = GenericInvId | BuildInvId | SwarmingTaskInvId;

export interface GenericInvId {
  readonly type: 'generic';
  readonly invId: string;
}

export interface BuildInvId {
  readonly type: 'build';
  readonly buildId: string;
}

export interface SwarmingTaskInvId {
  readonly type: 'swarming-task';
  readonly swarmingHost: string;
  readonly taskId: string;
}

/**
 * Parses the invocation and returns
 * 1. a `BuildInvId` if it's a buildbucket build invocation, or
 * 2. a `SwarmingTaskInvId` if it's a swarming task invocation, or
 * 3. a `GenericInvId` otherwise.
 *
 * Swarming task invocation must have a swarming host listed in the
 * `ALLOWED_SWARMING_HOSTS`.
 */
export function parseInvId(invId: string): ParsedInvId {
  // There's an alternative format for build invocation:
  // `build-${builderIdHash}-${buildNum}`.
  // We don't match that because:
  // 1. we can't get back the build link because the builderID is hashed, and
  // 2. typically those invocations are only used as wrapper invocations that
  //    points to the `build-${buildId}` for the same build for speeding up
  //    queries when buildId is not yet known to the client. We don't expect
  //    them to be used anywhere else.
  const buildId = invId.match(/^build-(?<id>\d+)/)?.groups?.['id'];
  if (buildId) {
    return {
      type: 'build',
      buildId,
    };
  }

  const { swarmingHost, taskId } =
    invId.match(/^task-(?<swarmingHost>.*)-(?<taskId>[0-9a-fA-F]+)$/)?.groups ||
    {};
  if (swarmingHost && taskId && ALLOWED_SWARMING_HOSTS.includes(swarmingHost)) {
    return {
      type: 'swarming-task',
      swarmingHost,
      taskId,
    };
  }

  return {
    type: 'generic',
    invId,
  };
}
