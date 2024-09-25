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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Chip,
  Tooltip,
  Typography,
} from '@mui/material';

import { AlertJson, TreeJson, Bug } from '@/monitoringv2/util/server_json';

import { AlertTable } from '../../components/alert_table';

interface AlertGroupProps {
  tree: TreeJson;
  alerts: AlertJson[];
  hiddenAlertsCount: number;
  groupName: string;
  groupDescription: string;
  defaultExpanded: boolean;
  bugs: Bug[];
}
// A collapsible group of alerts like 'consistent failures' or 'new failures'.
// Similar to BugGroup, but is never associated with a bug.
export const AlertGroup = ({
  tree,
  alerts,
  hiddenAlertsCount,
  groupName,
  groupDescription,
  defaultExpanded,
  bugs,
}: AlertGroupProps) => {
  return (
    <Accordion defaultExpanded={defaultExpanded}>
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        sx={{ '.MuiAccordionSummary-content': { alignItems: 'baseline' } }}
      >
        <Tooltip
          title={`There are ${
            alerts ? alerts.length : 0
          } ${groupName} not linked to any bugs`}
        >
          <Chip
            sx={{ marginRight: '8px' }}
            label={alerts ? alerts.length : 0}
            variant="outlined"
            color={alerts && alerts.length > 0 ? 'primary' : undefined}
          />
        </Tooltip>
        <Typography>
          {` ${groupName} `}
          <small css={{ opacity: '50%' }}>{groupDescription}</small>
        </Typography>
      </AccordionSummary>
      <AccordionDetails>
        {alerts.length > 0 ? (
          <>
            <AlertTable alerts={alerts} tree={tree} bugs={bugs} />
            {hiddenAlertsCount > 0 ? (
              <Typography
                sx={{ opacity: '60%', marginTop: '20px', paddingLeft: '32px' }}
              >
                {hiddenAlertsCount} alert{hiddenAlertsCount > 1 ? 's' : ''}{' '}
                hidden by the current filter
              </Typography>
            ) : null}
          </>
        ) : hiddenAlertsCount > 0 ? (
          <Typography sx={{ opacity: '60%', paddingLeft: '32px' }}>
            {hiddenAlertsCount} alert{hiddenAlertsCount > 1 ? 's' : ''} hidden
            by the current filter
          </Typography>
        ) : (
          <Typography sx={{ opacity: '60%', paddingLeft: '32px' }}>
            No alerts are currently in the {groupName} group.
          </Typography>
        )}
      </AccordionDetails>
    </Accordion>
  );
};
