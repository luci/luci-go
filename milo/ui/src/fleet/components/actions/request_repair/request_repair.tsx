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

import { DutToRepair } from '../shared/types';

interface RequestRepairProps {
  selectedDuts: DutToRepair[];
  // This is optional since it's only ever modified here
}

const REPAIR_COMPONENT_ID = '575445';
const REPAIR_TEMPLATE_ID = '1509031';
const LOCATION_MAP: { [key: string]: string[] } = {
  EM25: ['chromeos8'],
  '946': ['chromeos15', 'chromeos7', 'chromeos5', 'chromeos3'],
};

export const generateIssueDescription = (dutInfo: string) => {
  const description = [
    'Fleet Operations will repair DUTS that are in "needs_manual_repair" status. ' +
      'repair_failed & needs_repair are being recovered by auto repair tasks. ' +
      'Please do not explicitly assign bugs to individuals without prior ' +
      'discussion with said individuals',
    '------------------------------------------------------------------------------------',
    'Location key (zone - location):',
    '',
    '---------------------------------------------',
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

const getLocationForDut = (dutName: string): string => {
  for (const [location, prefixes] of Object.entries(LOCATION_MAP)) {
    if (prefixes.some((prefix) => dutName.includes(prefix))) {
      return location;
    }
  }

  return '<Please add if known>';
};

export const RequestRepair: React.FC<RequestRepairProps> = ({
  selectedDuts,
}) => {
  const dutInfo = selectedDuts.map((dut) => {
    return ` * http://go/fcdut/${dut.name} (Location: ${getLocationForDut(
      dut.name,
    )})`;
  });

  const fileDutRepairRequest = () => {
    const description = generateIssueDescription(dutInfo.join('\n'));
    const url = `http://b/issues/new?component=${REPAIR_COMPONENT_ID}&template=${REPAIR_TEMPLATE_ID}&description=${description}&markdown=true`;
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
