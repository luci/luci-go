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

import { md, rawMd } from '@/fleet/utils/markdown_utils';

import { DutToRepair } from '../shared/types';

import { RepairConfig } from './request_repair';
import { FLEET_CONSOLE_TRACKING_HOTLIST } from './request_repair_utils';

const LOCATION_MAP: { [key: string]: string[] } = {
  EM25: ['chromeos8'],
  EM24: ['chromeos6'],
  EM23: ['chromeos4', 'chromeos-rack4'],
  EM22: ['chromeos2', 'chromeos-rack2'],
  EM21: ['chromeos1', 'chromeos-rack1'],
  MTV1: ['chromeos15'],
};

const getLocationForDut = (name: string): string => {
  for (const [location, prefixes] of Object.entries(LOCATION_MAP)) {
    if (prefixes.some((prefix) => name.startsWith(prefix))) {
      return location;
    }
  }
  return '';
};

const generateIssueTitle = (duts: DutToRepair[]): string => {
  if (!duts.length) throw new Error('No DUTs specified');

  const dutName = duts[0].name;
  const board = duts[0].board;
  const model = duts[0].model;
  const pool = duts[0].pool;
  const extraDuts = duts.length > 1 ? ` and ${duts.length - 1} more` : '';
  const location = getLocationForDut(dutName) || 'Location Unknown';
  return `[${location}][Repair][${board}.${model}] Pool: [${pool}] [${dutName}]${extraDuts}`;
};

const generateDutInfo = (selectedDuts: DutToRepair[]): string =>
  selectedDuts
    .map(({ name, board, model, pool }) => {
      const extraInfo = [
        `Location: ${getLocationForDut(name) || '<Please add if known>'}`,
        board && `Board: ${board}`,
        model && `Model: ${model}`,
        pool && `Pool: ${pool}`,
      ];
      const dutUrl = rawMd(
        `http://goto.google.com/fcdut/${encodeURIComponent(name)}`,
      );
      return md` * [http://goto.google.com/fcdut/${name}](${dutUrl}) (${extraInfo.filter(Boolean).join(', ')})`;
    })
    .join('\n');

const generateIssueDescription = (dutInfo: string) => {
  const description = [
    'Fleet Operations will repair DUTS that are in "needs_manual_repair" status. ' +
      'repair_failed & needs_repair are being recovered by auto repair tasks. ' +
      'You can submit a bug for DUTs in a different state if you suspect that the state is incorrect, ' +
      'or something is wrong with devices peripherals. ' +
      'Please do not explicitly assign bugs to individuals without prior ' +
      'discussion with said individuals.',
    '------------------------------------------------------------------------------------',
    '',
    '**DUT Link(s) / Locations:**:',
    '',
    `${dutInfo}`,
    '',
    '**Issue:**',
    '',
    '<Please provide a summary of the issue.>',
    '',
    '**Action Item:**',
    '',
    '**Logs (if applicable):**',
  ].join('\n');
  return description;
};

export const CHROMEOS_BOARD_CUSTOM_FIELD_ID = '994275';
export const CHROMEOS_MODEL_CUSTOM_FIELD_ID = '994317';
export const CHROMEOS_QTY_CUSTOM_FIELD_ID = '994318';
export const CHROMEOS_HOST_NAME_CUSTOM_FIELD_ID = '998417';
export const CHROMEOS_SERVO_USB_STATE_CUSTOM_FIELD_ID = '1002657';
export const CHROMEOS_DUT_STATUS_CUSTOM_FIELD_ID = '1002699';
export const CHROMEOS_SERVO_STATUS_CUSTOM_FIELD_ID = '1002733';
export const CHROMEOS_POOL_CUSTOM_FIELD_ID = '1061740';
export const CHROMEOS_DUT_ASSET_CUSTOM_FIELD_ID = '1180197';
export const CHROMEOS_SERVO_SERIAL_CUSTOM_FIELD_ID = '1180248';

// Automated repair tracking matches single-DUT tickets by exact Host Name
// (998417) against DUTs in manual-repair states, and auto-closes NEW/ASSIGNED
// tickets when Host Name does not match a single failed DUT.
const MANUAL_REPAIR_HOST_NAME_STATES = new Set([
  'needs_manual_repair',
  'needs_replacement',
]);

const joinUniqueCustomFieldValues = (
  values: (string | undefined)[],
  excludeZero = false,
): string => {
  const unique: string[] = [];
  const seen = new Set<string>();
  for (const raw of values) {
    if (!raw) continue;
    for (const part of raw.split(',')) {
      const trimmed = part.trim();
      if (
        !trimmed ||
        trimmed === 'undefined' ||
        (excludeZero && trimmed === '0')
      ) {
        continue;
      }
      if (!seen.has(trimmed)) {
        seen.add(trimmed);
        unique.push(trimmed);
      }
    }
  }
  return unique.join(', ');
};

const generateChromeOSCustomFields = (duts: DutToRepair[]): string[] => {
  if (duts.length === 0) return [];
  const fields: string[] = [];
  const singleDut = duts.length === 1 ? duts[0] : undefined;
  const singleHostName = singleDut?.name?.trim();
  const shouldIncludeHostName =
    Boolean(singleHostName) &&
    singleHostName !== 'undefined' &&
    Boolean(
      singleDut?.state &&
        MANUAL_REPAIR_HOST_NAME_STATES.has(
          singleDut.state.trim().toLowerCase(),
        ),
    );

  const dutAssets = joinUniqueCustomFieldValues(
    duts.map((d) => d.dutId),
    true,
  );
  const boards = joinUniqueCustomFieldValues(duts.map((d) => d.board));
  const models = joinUniqueCustomFieldValues(duts.map((d) => d.model));
  const pools = joinUniqueCustomFieldValues(duts.map((d) => d.pool));
  const states = joinUniqueCustomFieldValues(duts.map((d) => d.state));
  const servoStates = joinUniqueCustomFieldValues(
    duts.map((d) => d.servoState),
  );
  const servoUsbStates = joinUniqueCustomFieldValues(
    duts.map((d) => d.servoUsbState),
  );
  const servoSerials = joinUniqueCustomFieldValues(
    duts.map((d) => d.servoSerial),
  );

  if (shouldIncludeHostName) {
    fields.push(`${CHROMEOS_HOST_NAME_CUSTOM_FIELD_ID}:${singleHostName}`);
  }
  if (dutAssets) {
    fields.push(`${CHROMEOS_DUT_ASSET_CUSTOM_FIELD_ID}:${dutAssets}`);
  }
  if (boards) {
    fields.push(`${CHROMEOS_BOARD_CUSTOM_FIELD_ID}:${boards}`);
  }
  if (models) {
    fields.push(`${CHROMEOS_MODEL_CUSTOM_FIELD_ID}:${models}`);
  }
  if (pools) {
    fields.push(`${CHROMEOS_POOL_CUSTOM_FIELD_ID}:${pools}`);
  }
  if (states) {
    fields.push(`${CHROMEOS_DUT_STATUS_CUSTOM_FIELD_ID}:${states}`);
  }
  if (servoStates) {
    fields.push(`${CHROMEOS_SERVO_STATUS_CUSTOM_FIELD_ID}:${servoStates}`);
  }
  if (servoUsbStates) {
    fields.push(
      `${CHROMEOS_SERVO_USB_STATE_CUSTOM_FIELD_ID}:${servoUsbStates}`,
    );
  }
  if (servoSerials) {
    fields.push(`${CHROMEOS_SERVO_SERIAL_CUSTOM_FIELD_ID}:${servoSerials}`);
  }
  fields.push(`${CHROMEOS_QTY_CUSTOM_FIELD_ID}:${duts.length}`);
  return fields;
};

export const ChromeOSRepairConfig: RepairConfig<DutToRepair> = {
  componentId: '575445',
  getTemplateId: () => '1509031',
  generateTitle: generateIssueTitle,
  generateDescription: (items) =>
    generateIssueDescription(generateDutInfo(items)),
  hotlistIds: FLEET_CONSOLE_TRACKING_HOTLIST,
  customFields: generateChromeOSCustomFields,
};
