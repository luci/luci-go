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

import { BuildIdentifier, USING_BUILD_BUCKET_DEV } from '@/fleet/utils/builds';
import {
  BatchRequest_Request,
  BatchResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';

// Based on: https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/infra/libs/skylab/buildbucket/bb.go;l=26
export const TASK_PRIORITY = 4;

// See: https://source.chromium.org/chromium/_/chromium/infra/infra/+/f9b80a6c0bb478f5fb0a3b813579088408ca18dc:go/src/infra/libs/skylab/buildbucket/tasknames.go;l=27
export const REPAIR_TASK_NAME = 'recovery';
export const REPAIR_DEEP_TASK_NAME = 'deep_recovery';

const PROD_BUILDER = {
  project: 'chromeos',
  bucket: 'labpack_runner',
  builder: 'repair',
};

// When calling Buildbucket dev, the typical builders we run autorepair against
// don't exist, so we test against a fake builder that should run a no op.
const DEV_BUILDER = {
  project: 'infra',
  bucket: 'ci',
  builder: 'linux-rel-buildbucket',
};

export interface DutNameAndState {
  name: string;
  state?: string;
}

/**
 * Generates a set of batched requests to BuildBucket based on dut names.
 *
 * @param dutNames List of DUTS to run autorepair against.
 * @param sessionId A custom unique string identifying the admin session the
 *      user is in. Used for users to be able to look up all tasks run in this
 *      session.
 * @returns A list of requests for Buildbucket's batch endpoint.
 */
export function autorepairRequestsFromDuts(
  duts: DutNameAndState[],
  sessionId: string,
  deep: boolean = false,
): BatchRequest_Request[] {
  // Combine all build requests into one Swarming view using a session ID.

  const builder = USING_BUILD_BUCKET_DEV ? DEV_BUILDER : PROD_BUILDER;

  return duts.map((dut) => {
    // Avoid matching dut_name on Buildbucket dev because duts do not exist
    // on dev.
    const variableDimensions = USING_BUILD_BUCKET_DEV
      ? []
      : [
          {
            key: 'dut_name',
            value: `${dut.name}`,
          },
        ];

    const dimensions = [
      ...variableDimensions,
      // We want to use the initial dut_state as one of our scheduling
      // dimensions because, otherwise, it's possible for the dut_state in the
      // UI to be different than it becomes at the time of scheduling.
      {
        key: 'dut_state',
        value: dut.state,
      },
    ];

    return BatchRequest_Request.fromPartial({
      scheduleBuild: {
        requestId: `${sessionId}-${dut.name}`,
        builder,
        priority: TASK_PRIORITY,
        dimensions,
        tags: [
          {
            key: 'dut-name',
            value: `${dut.name}`,
          },
          {
            key: 'admin-session',
            value: sessionId,
          },
          {
            key: 'task',
            value: deep ? REPAIR_DEEP_TASK_NAME : REPAIR_TASK_NAME,
          },
          {
            key: 'client',
            value: 'fleet-console',
          },
          {
            key: 'version',
            value: 'prod',
          },
          {
            key: 'qs_account',
            value: 'unmanaged_p0',
          },
        ],
      },
    });
  });
}

export function extractBuildIdentifiers(
  builds: BatchResponse,
): BuildIdentifier[] {
  return builds.responses.map(({ scheduleBuild }) => ({
    ...scheduleBuild?.builder,
    buildId: scheduleBuild?.id,
  }));
}
