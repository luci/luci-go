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
  ScheduleAutorepairRequest,
  ScheduleAutorepairRequest_AutorepairFlag,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import {
  BatchRequest_Request,
  BatchResponse,
} from '@/proto/go.chromium.org/luci/buildbucket/proto/builds_service.pb';
import { StringPair } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

import { DutToRepair } from '../../actions/shared/types';

// Based on: https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/infra/libs/skylab/buildbucket/bb.go;l=26
export const TASK_PRIORITY = 4;

// See: https://source.chromium.org/chromium/_/chromium/infra/infra/+/f9b80a6c0bb478f5fb0a3b813579088408ca18dc:go/src/infra/libs/skylab/buildbucket/tasknames.go;l=27
export const REPAIR_TASK_NAME = 'recovery';
export const REPAIR_DEEP_TASK_NAME = 'deep_recovery';

/**
 * Canonical ChromeOS UFS / PARIS targeted repair actions (`ufslab.DutState_RepairRequest`
 * in `infra/unifiedfleet/api/v1/models/chromeos/lab/dut_state.proto`) currently
 * supported by Fleet Console autorepair.
 */
export type RepairRequestAction =
  | 'PROVISION'
  | 'UPDATE_USBKEY_IMAGE'
  | 'REIMAGE_BY_USBKEY'
  | 'AUDIT_STORAGE'
  | 'AUDIT_USBKEY';

export interface RepairRequestOption {
  readonly value: RepairRequestAction;
  readonly label: string;
}

export const REPAIR_REQUEST_OPTIONS: readonly RepairRequestOption[] = [
  {
    value: 'PROVISION',
    label: 'Re-provision DUT',
  },
  {
    value: 'UPDATE_USBKEY_IMAGE',
    label: 'Update USB key image',
  },
  {
    value: 'REIMAGE_BY_USBKEY',
    label: 'Re-image from USB',
  },
  {
    value: 'AUDIT_STORAGE',
    label: 'Audit internal storage',
  },
  {
    value: 'AUDIT_USBKEY',
    label: 'Audit USB drive',
  },
];

export interface AutorepairCliCommandOptions {
  readonly deepRepair?: boolean;
  readonly latestRepair?: boolean;
  readonly verifyOnly?: boolean;
  readonly isPartner?: boolean;
  readonly repairRequests?: readonly RepairRequestAction[];
}

/**
 * Formats the equivalent `shivas` CLI command(s). When targeted repair
 * requests are selected, `shivas update dut-state -dut-name <dut> -repair-request <REQS>`
 * is emitted for each DUT before the `shivas repair` command, because PARIS
 * reads `dut_state.repair_requests` from UFS.
 */
export function formatShivasRepairCommand(
  dutNames: readonly string[],
  options: AutorepairCliCommandOptions = {},
): string {
  const {
    deepRepair = false,
    latestRepair = false,
    verifyOnly = false,
    isPartner = false,
    repairRequests = [],
  } = options;
  const namespaceFlag = isPartner ? ' -namespace=os-partner' : '';
  const repairCmd = `shivas repair${deepRepair ? ' -deep' : ''}${
    latestRepair ? ' -latest' : ''
  }${verifyOnly ? ' -verify' : ''}${namespaceFlag} ${dutNames.join(' ')}`;

  if (repairRequests.length === 0 || dutNames.length === 0) {
    return repairCmd;
  }

  const joinedRequests = repairRequests.join(',');
  const updateCmds = dutNames.map(
    (dut) =>
      `shivas update dut-state -dut-name ${dut} -repair-request ${joinedRequests}${namespaceFlag}`,
  );
  return [...updateCmds, repairCmd].join('\n');
}

/**
 * Constructs a `ScheduleAutorepairRequest` carrying optional `repairRequests`.
 */
export function createScheduleAutorepairRequest(
  unitNames: readonly string[],
  flags: readonly ScheduleAutorepairRequest_AutorepairFlag[],
  repairRequests: readonly RepairRequestAction[] = [],
): ScheduleAutorepairRequest {
  return ScheduleAutorepairRequest.fromPartial({
    unitNames: [...unitNames],
    flags: [...flags],
    repairRequests: [...repairRequests],
  });
}

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

/**
 * Generates a set of batched requests to BuildBucket based on dut names.
 *
 * @param duts List of DUTs to run autorepair against.
 * @param sessionId A custom unique string identifying the admin session the
 *      user is in. Used for users to be able to look up all tasks run in this
 *      session.
 * @param deep Whether to run a deep repair workflow.
 * @param tags Additional Buildbucket/Swarming tags.
 * @param repairRequests Optional targeted RepairRequest actions passed via
 *      Buildbucket input properties (`repair_requests`) and Swarming tags
 *      (`repair_request:<ACTION>`).
 * @returns A list of requests for Buildbucket's batch endpoint.
 */
export function autorepairRequestsFromDuts(
  duts: DutToRepair[],
  sessionId: string,
  deep: boolean = false,
  tags: readonly StringPair[] = [],
  repairRequests: readonly RepairRequestAction[] = [],
): BatchRequest_Request[] {
  // Combine all build requests into one Swarming view using a session ID.

  const builder = USING_BUILD_BUCKET_DEV ? DEV_BUILDER : PROD_BUILDER;
  const repairRequestTags: StringPair[] = repairRequests.map((action) => ({
    key: 'repair_request',
    value: action,
  }));

  return duts.map((dut) => {
    // Avoid matching dut_name on Buildbucket dev because duts do not exist
    // on dev.
    const variableDimensions = USING_BUILD_BUCKET_DEV
      ? []
      : [
          {
            key: 'dut_id',
            value: `${dut.dutId}`,
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

    const properties: Record<string, unknown> = {
      admin_service: 'chromeos-skylab-bot-fleet.appspot.com',
      configuration: '',
      disable_cft: false,
      enable_recovery: true,
      inventory_namespace: 'os',
      inventory_service: 'ufs.api.cr.dev',
      no_metrics: false,
      no_stepper: false,
      task_name: 'recovery',
      unit_name: dut.name,
      update_inventory: true,
    };
    if (repairRequests.length > 0) {
      properties.repair_requests = [...repairRequests];
    }

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
            key: 'dut-id',
            value: `${dut.dutId}`,
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
          ...repairRequestTags,
          ...tags,
        ],
        // Required for autorepair to run successfully. See: b/401004801
        properties,
      },
    });
  });
}

export function extractBuildIdentifiers(
  builds: BatchResponse,
): BuildIdentifier[] {
  return builds.responses.map(({ scheduleBuild }) => ({
    ...scheduleBuild?.builder,
    buildId: scheduleBuild ? `b${scheduleBuild.id}` : '',
  }));
}
