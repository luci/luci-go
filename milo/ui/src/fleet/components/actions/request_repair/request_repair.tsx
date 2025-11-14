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
import { FeedbackOutlined } from '@mui/icons-material';
import { Button } from '@mui/material';
import React from 'react';

import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

import { DutToRepair } from '../shared/types';

interface RequestRepairProps {
  selectedDuts: DutToRepair[];
}

const TITLE_LOCATION_PLACEHOLDER = 'Location Unknown';
const DESCRIPTION_LOCATION_PLACEHOLDER = '<Please add if known>';
const REPAIR_COMPONENT_ID = '575445';
const REPAIR_TEMPLATE_ID = '1509031';
const TRACKING_HOTLIST_ID = '7555487';
const LOCATION_MAP: { [key: string]: string[] } = {
  EM25: ['chromeos8'],
  '946': ['chromeos15', 'chromeos7', 'chromeos5', 'chromeos3'],
};

const getLocationForDut = (dutName: string): string | undefined => {
  for (const [lab, prefixes] of Object.entries(LOCATION_MAP)) {
    if (prefixes.some((prefix) => dutName.includes(prefix))) {
      return lab;
    }
  }

  return undefined;
};

const generateIssueTitle = (duts: DutToRepair[]): string => {
  if (!duts.length) throw new Error('No DUTs specified');

  const dutName = duts[0].name;
  const board = duts[0].board;
  const model = duts[0].model;
  const pool = duts[0].pool;
  const extraDuts = duts.length > 1 ? ` and ${duts.length - 1} more` : '';
  const location = getLocationForDut(dutName) || TITLE_LOCATION_PLACEHOLDER;
  return `[${location}][Repair][${board}.${model}] Pool: [${pool}] [${dutName}]${extraDuts}`;
};

export const generateIssueDescription = (dutInfo: string) => {
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
  return encodeURIComponent(description);
};

const generateDutInfo = (selectedDuts: DutToRepair[]): string =>
  selectedDuts
    .map(({ name, board, model, pool }) => {
      const extraInfo = [
        `Location: ${getLocationForDut(name) || DESCRIPTION_LOCATION_PLACEHOLDER}`,
        board && `Board: ${board}`,
        model && `Model: ${model}`,
        pool && `Pool: ${pool}`,
      ];
      return ` * http://go/fcdut/${name} (${extraInfo.join(', ')})`;
    })
    .join('\n');

export const RequestRepair: React.FC<RequestRepairProps> = ({
  selectedDuts,
}) => {
  const { trackEvent } = useGoogleAnalytics();
  const showButton = selectedDuts?.length > 0;

  if (!showButton) {
    return <></>;
  }

  const fileDutRepairRequest = () => {
    trackEvent('request_repair', {
      componentName: 'request_repair_button',
      selectedDuts: selectedDuts.length,
    });
    const dutInfo = generateDutInfo(selectedDuts);
    const description = generateIssueDescription(dutInfo);
    const title = encodeURIComponent(generateIssueTitle(selectedDuts));
    const url = `http://b/issues/new?markdown=true&component=${REPAIR_COMPONENT_ID}&template=${REPAIR_TEMPLATE_ID}&title=${title}&description=${description}&hotlistIds=${TRACKING_HOTLIST_ID}`;
    window.open(url, '_blank');
  };
  return (
    <Button
      data-testid="file-repair-bug-button"
      onClick={fileDutRepairRequest}
      color="primary"
      size="small"
      startIcon={<FeedbackOutlined />}
    >
      Request Repair
    </Button>
  );
};
