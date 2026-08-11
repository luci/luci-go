// Copyright 2026 The LUCI Authors.
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

import { DeviceConfigEdits } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/chromeos.pb';
import { oSRPM_TypeToJSON } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/chromeos/lab/rpm.pb';
import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import { formatEnum } from './formatters';

export const isLabstationConfig = (
  lse: MachineLSE | null | undefined,
): boolean => {
  return Boolean(lse?.chromeosMachineLse?.deviceLse?.labstation);
};

export const MAX_FIELD_LENGTH = 100;
export const MAX_NUMBER_INPUT_LENGTH = 15;
export const MAX_ARRAY_TOTAL_LENGTH = 2 * MAX_FIELD_LENGTH;

export const checkLengthLimit = (
  value: string | null | undefined,
  limit = MAX_FIELD_LENGTH,
): boolean => {
  if (!value) {
    return true;
  }
  return value.length <= limit;
};

export const LOGICAL_SCHEDULING_PATHS = {
  dutPools: 'chromeosMachineLse.deviceLse.dut.pools',
  labstationPools: 'chromeosMachineLse.deviceLse.labstation.pools',
};

export const SERVO_PATHS = {
  hostname: 'chromeosMachineLse.deviceLse.dut.peripherals.servo.servoHostname',
  port: 'chromeosMachineLse.deviceLse.dut.peripherals.servo.servoPort',
  serial: 'chromeosMachineLse.deviceLse.dut.peripherals.servo.servoSerial',
};

export const LOCATION_PATHS = {
  zone: 'zone',
  rack: 'rack',
};

export const RPM_PATHS = {
  dut: {
    host: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.powerunitName',
    outlet: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.powerunitOutlet',
    type: 'chromeosMachineLse.deviceLse.dut.peripherals.rpm.powerunitType',
  },
  labstation: {
    host: 'chromeosMachineLse.deviceLse.labstation.rpm.powerunitName',
    outlet: 'chromeosMachineLse.deviceLse.labstation.rpm.powerunitOutlet',
    type: 'chromeosMachineLse.deviceLse.labstation.rpm.powerunitType',
  },
};

export interface FieldConfig {
  label: string;
  path: string;
  editPath: string;
  type: 'string' | 'number' | 'array';
  requiresRedeploy?: boolean;
  min?: number;
  max?: number;
  formatter?: (val: unknown) => string;
}

export const getEditableFields = (isLabstation: boolean): FieldConfig[] => {
  const rpmPaths = isLabstation ? RPM_PATHS.labstation : RPM_PATHS.dut;
  const fields: FieldConfig[] = [
    {
      label: 'Pools',
      path: isLabstation
        ? LOGICAL_SCHEDULING_PATHS.labstationPools
        : LOGICAL_SCHEDULING_PATHS.dutPools,
      editPath: 'pools',
      type: 'array',
    },
    {
      label: 'Zone',
      path: LOCATION_PATHS.zone,
      editPath: 'zone',
      type: 'string',
      requiresRedeploy: false,
    },
    {
      label: 'Rack',
      path: LOCATION_PATHS.rack,
      editPath: 'rack',
      type: 'string',
      requiresRedeploy: false,
    },
    {
      label: 'RPM Hostname',
      path: rpmPaths.host,
      editPath: 'rpm.host',
      type: 'string',
      requiresRedeploy: true,
    },
    {
      label: 'RPM Outlet',
      path: rpmPaths.outlet,
      editPath: 'rpm.outlet',
      type: 'string',
      requiresRedeploy: true,
    },
    {
      label: 'RPM Type',
      path: rpmPaths.type,
      editPath: 'rpm.type',
      type: 'number',
      requiresRedeploy: true,
      formatter: (val) => formatEnum(val as number, oSRPM_TypeToJSON, 'TYPE_'),
    },
  ];
  if (!isLabstation) {
    fields.push(
      {
        label: 'Servo Hostname',
        path: SERVO_PATHS.hostname,
        editPath: 'servo.hostname',
        type: 'string',
        requiresRedeploy: true,
      },
      {
        label: 'Servo Port',
        path: SERVO_PATHS.port,
        editPath: 'servo.port',
        type: 'number',
        requiresRedeploy: true,
        min: 9000,
        max: 9999,
      },
      {
        label: 'Servo Serial',
        path: SERVO_PATHS.serial,
        editPath: 'servo.serial',
        type: 'string',
        requiresRedeploy: true,
      },
    );
  }
  return fields;
};

export const getSegments = (path: string | string[]): string[] => {
  return typeof path === 'string' ? path.split('.') : path;
};

export const getNestedValue = (
  obj: unknown,
  path: string | string[],
): unknown => {
  if (!obj || typeof obj !== 'object') return undefined;
  const segments = getSegments(path);
  let curr = obj as Record<string, unknown>;
  for (const key of segments) {
    if (curr === null || curr === undefined || typeof curr !== 'object') {
      return undefined;
    }
    curr = curr[key] as Record<string, unknown>;
  }
  return curr;
};

export const mutateNestedValue = (
  obj: Record<string, unknown>,
  path: string | string[],
  value: unknown,
): void => {
  const segments = getSegments(path);
  let curr = obj;
  for (let i = 0; i < segments.length - 1; i++) {
    const key = segments[i];
    if (!(key in curr) || typeof curr[key] !== 'object' || curr[key] === null) {
      curr[key] = {};
    }
    curr = curr[key] as Record<string, unknown>;
  }
  const lastKey = segments[segments.length - 1];
  curr[lastKey] = value;
};

export const updateNestedValues = (
  obj: Record<string, unknown>,
  updates: Array<{ path: string | string[]; value: unknown }>,
): Record<string, unknown> => {
  // Deep copy using JSON serialization. This is safe for plain proto-based objects
  // (without functions/Dates) and avoids structuredClone compatibility issues
  // during Jest execution in Node test environments.
  const copy = JSON.parse(JSON.stringify(obj));
  updates.forEach(({ path, value }) => {
    mutateNestedValue(copy, path, value);
  });
  return copy;
};

export const areArraysEqual = (a: string[], b: string[]): boolean => {
  if (a.length !== b.length) return false;
  const aSorted = [...a].sort();
  const bSorted = [...b].sort();
  return aSorted.every((val, idx) => val === bSorted[idx]);
};

export interface FieldDiff {
  path: string;
  original: string;
  updated: string;
}

export const calculateDiff = (
  original: MachineLSE | null | undefined,
  updated: MachineLSE | null | undefined,
): FieldDiff[] => {
  const diffs: FieldDiff[] = [];
  if (!original || !updated) return diffs;

  const isLabstation = isLabstationConfig(original);
  const fields = getEditableFields(isLabstation);

  fields.forEach(({ label, path, type, formatter }) => {
    const origVal = getNestedValue(original, path);
    const draftVal = getNestedValue(updated, path);

    if (type === 'array') {
      const origArr = Array.isArray(origVal) ? (origVal as string[]) : [];
      const draftArr = Array.isArray(draftVal) ? (draftVal as string[]) : [];
      if (!areArraysEqual(origArr, draftArr)) {
        diffs.push({
          path: label,
          original: origArr.join(',') || '(empty)',
          updated: draftArr.join(',') || '(empty)',
        });
      }
    } else {
      let origStr = formatter
        ? formatter(origVal)
        : origVal !== undefined && origVal !== null
          ? String(origVal)
          : '';
      let draftStr = formatter
        ? formatter(draftVal)
        : draftVal !== undefined && draftVal !== null
          ? String(draftVal)
          : '';
      if (type === 'number') {
        if (origStr === '0') origStr = '';
        if (draftStr === '0') draftStr = '';
      }
      if (origStr !== draftStr) {
        diffs.push({
          path: label,
          original: origStr || '(empty)',
          updated: draftStr || '(empty)',
        });
      }
    }
  });

  return diffs;
};

export const translateDiffToEdits = (
  original: MachineLSE | null | undefined,
  updated: MachineLSE | null | undefined,
): { edits: Partial<DeviceConfigEdits>; paths: string[] } => {
  const edits: Record<string, unknown> = {};
  const paths: string[] = [];
  if (!original || !updated) return { edits: {}, paths };

  const isLabstation = isLabstationConfig(original);
  const fields = getEditableFields(isLabstation);

  fields.forEach(({ path, editPath, type }) => {
    const origVal = getNestedValue(original, path);
    const draftVal = getNestedValue(updated, path);

    let hasChanged = false;
    if (type === 'array') {
      const origArr = Array.isArray(origVal) ? (origVal as string[]) : [];
      const draftArr = Array.isArray(draftVal) ? (draftVal as string[]) : [];
      hasChanged = !areArraysEqual(origArr, draftArr);
    } else if (type === 'number') {
      const origNum = !origVal || Number(origVal) === 0 ? 0 : Number(origVal);
      const draftNum =
        !draftVal || Number(draftVal) === 0 ? 0 : Number(draftVal);
      hasChanged = origNum !== draftNum;
    } else {
      hasChanged = origVal !== draftVal;
    }

    if (hasChanged) {
      let val: unknown = type === 'array' ? (draftVal ?? []) : draftVal;
      if (type === 'number' && (!val || Number(val) === 0)) {
        val = 0;
      } else if (type === 'number') {
        val = Number(val);
      } else if (type === 'string' && (val === null || val === undefined)) {
        val = '';
      }
      if (editPath === 'rpm.type') {
        const rawStr = oSRPM_TypeToJSON(val as number);
        val = rawStr.replace('TYPE_', '').toLowerCase();
        if (val === 'unknown') {
          val = '';
        }
      }
      mutateNestedValue(edits, editPath, val);
      paths.push(editPath);
    }
  });

  if (edits.servo && (edits.servo as Record<string, unknown>).hostname === '') {
    (edits.servo as Record<string, unknown>).port = 0;
    if (!paths.includes('servo.port')) {
      paths.push('servo.port');
    }
  }

  if (edits.rpm && (edits.rpm as Record<string, unknown>).host === '') {
    (edits.rpm as Record<string, unknown>).outlet = '';
    if (!paths.includes('rpm.outlet')) {
      paths.push('rpm.outlet');
    }
    const typeIdx = paths.indexOf('rpm.type');
    if (typeIdx > -1) {
      paths.splice(typeIdx, 1);
    }
    (edits.rpm as Record<string, unknown>).type = '';
  }

  // UFS requires updating rack whenever zone is updated ("Cannot update zone without updating rack").
  // Automatically include existing rack in edits if zone was modified.
  if (paths.includes('zone') && !paths.includes('rack')) {
    const rackVal = String(
      getNestedValue(updated, LOCATION_PATHS.rack) ||
        getNestedValue(original, LOCATION_PATHS.rack) ||
        '',
    );
    edits.rack = rackVal;
    paths.push('rack');
  }

  return { edits: edits as Partial<DeviceConfigEdits>, paths };
};

export const generateShivasCommands = (
  original: MachineLSE | null | undefined,
  updated: MachineLSE | null | undefined,
  hostname: string,
  ufsNamespace: string,
): string[] => {
  if (!original || !updated) return [];

  const commands: string[] = [];
  const isLabstation = isLabstationConfig(original);

  // DUT/Labstation/Machine updates
  const dutFlags: string[] = [];
  const labstationFlags: string[] = [];
  const machineFlags: string[] = [];

  // Pools
  const origPools =
    (getNestedValue(
      original,
      isLabstation
        ? LOGICAL_SCHEDULING_PATHS.labstationPools
        : LOGICAL_SCHEDULING_PATHS.dutPools,
    ) as string[]) || [];
  const updatedPools =
    (getNestedValue(
      updated,
      isLabstation
        ? LOGICAL_SCHEDULING_PATHS.labstationPools
        : LOGICAL_SCHEDULING_PATHS.dutPools,
    ) as string[]) || [];

  if (!areArraysEqual(origPools, updatedPools)) {
    const poolsVal = updatedPools.length === 0 ? '-' : updatedPools.join(',');
    if (isLabstation) {
      labstationFlags.push('-pools-replace', poolsVal);
    } else {
      dutFlags.push('-pools-replace', poolsVal);
    }
  }

  const origZone = String(getNestedValue(original, LOCATION_PATHS.zone) || '');
  const updatedZone = String(
    getNestedValue(updated, LOCATION_PATHS.zone) || '',
  );
  const origRack = String(getNestedValue(original, LOCATION_PATHS.rack) || '');
  const updatedRack = String(
    getNestedValue(updated, LOCATION_PATHS.rack) || '',
  );
  if (origZone !== updatedZone) {
    machineFlags.push('-zone', updatedZone || '-', '-rack', updatedRack || '-');
  } else if (origRack !== updatedRack) {
    machineFlags.push('-rack', updatedRack || '-');
  }

  if (!isLabstation) {
    const origServoHost = String(
      getNestedValue(original, SERVO_PATHS.hostname) || '',
    );
    const updatedServoHost = String(
      getNestedValue(updated, SERVO_PATHS.hostname) || '',
    );
    const origServoPortVal = getNestedValue(original, SERVO_PATHS.port);
    const updatedServoPortVal = getNestedValue(updated, SERVO_PATHS.port);
    const origServoPort =
      origServoPortVal && Number(origServoPortVal) > 0
        ? String(origServoPortVal)
        : '';
    const updatedServoPort =
      updatedServoPortVal && Number(updatedServoPortVal) > 0
        ? String(updatedServoPortVal)
        : '';

    const servoHostChanged = origServoHost !== updatedServoHost;
    const servoPortChanged = origServoPort !== updatedServoPort;

    if (servoHostChanged || servoPortChanged) {
      if (!updatedServoHost) {
        dutFlags.push('-servo', '-');
      } else {
        const portSuffix = updatedServoPort ? `:${updatedServoPort}` : '';
        dutFlags.push('-servo', `${updatedServoHost}${portSuffix}`);
      }
    }

    const origServoSerial = String(
      getNestedValue(original, SERVO_PATHS.serial) || '',
    );
    const updatedServoSerial = String(
      getNestedValue(updated, SERVO_PATHS.serial) || '',
    );
    if (origServoSerial !== updatedServoSerial) {
      dutFlags.push(
        '-servo-serial',
        updatedServoSerial ? updatedServoSerial : '-',
      );
    }
  }

  // RPM Host/Outlet/Type
  const rpmPaths = isLabstation ? RPM_PATHS.labstation : RPM_PATHS.dut;
  const origRpmHost = String(getNestedValue(original, rpmPaths.host) || '');
  const updatedRpmHost = String(getNestedValue(updated, rpmPaths.host) || '');
  const origRpmOutlet = String(getNestedValue(original, rpmPaths.outlet) || '');
  const updatedRpmOutlet = String(
    getNestedValue(updated, rpmPaths.outlet) || '',
  );
  const origRpmTypeVal = getNestedValue(original, rpmPaths.type);
  const updatedRpmTypeVal = getNestedValue(updated, rpmPaths.type);

  const rpmHostChanged = origRpmHost !== updatedRpmHost;
  const rpmOutletChanged = origRpmOutlet !== updatedRpmOutlet;
  const rpmTypeChanged = origRpmTypeVal !== updatedRpmTypeVal;

  const targetFlags = isLabstation ? labstationFlags : dutFlags;

  if (rpmHostChanged || rpmOutletChanged || rpmTypeChanged) {
    if (!updatedRpmHost) {
      targetFlags.push('-rpm', '-');
    } else {
      if (rpmHostChanged) {
        targetFlags.push('-rpm', updatedRpmHost);
      }
      if (rpmOutletChanged) {
        targetFlags.push(
          '-rpm-outlet',
          updatedRpmOutlet ? updatedRpmOutlet : '-',
        );
      }
      if (
        rpmTypeChanged &&
        updatedRpmTypeVal !== undefined &&
        updatedRpmTypeVal !== null
      ) {
        const typeStr = oSRPM_TypeToJSON(updatedRpmTypeVal as number)
          .replace('TYPE_', '')
          .toLowerCase();
        targetFlags.push('-rpm-type', typeStr);
      }
    }
  }

  // Combine DUT/Labstation flags into commands
  if (isLabstation && labstationFlags.length > 0) {
    const cmdParts = ['shivas', 'update', 'labstation', '-name', hostname];
    if (ufsNamespace) {
      cmdParts.push('-namespace', ufsNamespace);
    }
    cmdParts.push(...labstationFlags);
    commands.push(cmdParts.join(' '));
  } else if (!isLabstation && dutFlags.length > 0) {
    const cmdParts = ['shivas', 'update', 'dut', '-name', hostname];
    if (ufsNamespace) {
      cmdParts.push('-namespace', ufsNamespace);
    }
    cmdParts.push(...dutFlags);
    commands.push(cmdParts.join(' '));
  }

  if (machineFlags.length > 0) {
    const machineName =
      (original.machines && original.machines.length > 0
        ? original.machines[0]
        : undefined) || hostname;
    const cmdParts = ['shivas', 'update', 'machine', '-name', machineName];
    if (ufsNamespace) {
      cmdParts.push('-namespace', ufsNamespace);
    }
    cmdParts.push(...machineFlags);
    commands.push(cmdParts.join(' '));
  }

  return commands;
};

export const generateChangelogMarkdown = (
  diffs: FieldDiff[],
  deviceId: string,
): string => {
  if (diffs.length === 0) return '';
  const lines = [`**Inventory updates for ${deviceId}:**`];
  diffs.forEach((diff) => {
    lines.push(
      `*   **${diff.path}**: \`${diff.original}\` ➔ \`${diff.updated}\``,
    );
  });
  return lines.join('\n');
};

export const hasDeployableEdits = (
  diffs: FieldDiff[],
  isLabstation: boolean,
): boolean => {
  if (isLabstation) return false;
  const fields = getEditableFields(isLabstation);
  const deployableLabels = new Set(
    fields.filter((f) => f.requiresRedeploy).map((f) => f.label),
  );
  return diffs.some((d) => deployableLabels.has(d.path));
};
