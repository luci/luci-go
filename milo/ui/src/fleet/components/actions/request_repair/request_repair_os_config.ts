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
      const dutUrl = rawMd(`http://go/fcdut/${encodeURIComponent(name)}`);
      return md` * [http://go/fcdut/${name}](${dutUrl}) (${extraInfo.filter(Boolean).join(', ')})`;
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

export const ChromeOSRepairConfig: RepairConfig<DutToRepair> = {
  componentId: '575445',
  getTemplateId: () => '1509031',
  generateTitle: generateIssueTitle,
  generateDescription: (items) =>
    generateIssueDescription(generateDutInfo(items)),
  hotlistIds: FLEET_CONSOLE_TRACKING_HOTLIST,
};
