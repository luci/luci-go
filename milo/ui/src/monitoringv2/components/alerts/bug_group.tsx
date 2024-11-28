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

import BugReportIcon from '@mui/icons-material/BugReport';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Chip,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';

import { TreeJson, Bug } from '@/monitoringv2/util/server_json';

import { AlertTable } from '../../components/alert_table';

import { StructuredAlert } from './alert_tabs';

interface BugGroupProps {
  bug: Bug;
  alerts: StructuredAlert[];
  tree: TreeJson;
  bugs: Bug[];
}
/**
 * A collapsible group of failures that are associated with a bug.
 * Similar to AlertGroup, but displays bug information as well.
 */
export const BugGroup = ({ bug, alerts, tree, bugs }: BugGroupProps) => {
  return (
    <>
      <Accordion defaultExpanded={false}>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              width: '100%',
            }}
          >
            <Tooltip
              title={`There are ${
                alerts ? alerts.length : 0
              } alerts linked to this bug`}
            >
              <Chip
                sx={{ marginRight: '8px' }}
                label={alerts ? alerts.length : 0}
                variant="outlined"
                color={alerts && alerts.length > 0 ? 'primary' : undefined}
              />
            </Tooltip>
            <Typography sx={{ flexShrink: '1', flexGrow: '1' }}>
              {/* TODO: copy button because can't highlight the text here. */}{' '}
              <a
                href={bug.link}
                css={{ textDecoration: 'none' }}
                target="_blank"
                rel="noreferrer"
              >
                b/{bug.number}
              </a>{' '}
              {bug.summary !== undefined ? (
                bug.summary
              ) : (
                <span css={{ opacity: '50%' }}>
                  Unable to read bug information, please add the &quot;
                  {tree.bug_queue_label}&quot; label to the bug.
                </span>
              )}{' '}
              {bug.labels.map((l) => (
                <small key={l} css={{ opacity: '50%' }}>
                  {' '}
                  {l}{' '}
                </small>
              ))}
            </Typography>
            <div css={{ flexShrink: '0' }}>
              {bug.status !== undefined ? (
                <Chip label={<small>{bug.status}</small>} variant="outlined" />
              ) : null}
              {bug.priority !== undefined ? (
                <Chip
                  sx={{ marginLeft: '8px' }}
                  label={`P${bug.priority}`}
                  variant={bug.priority === 0 ? 'filled' : 'outlined'}
                  color={bug.priority < 2 ? 'primary' : undefined}
                />
              ) : null}
            </div>
          </div>
        </AccordionSummary>
        <AccordionDetails>
          {alerts && alerts.length ? (
            <AlertTable alerts={alerts} tree={tree} bugs={bugs} />
          ) : (
            <>
              <Typography sx={{ opacity: '50%' }}>
                No alerts are currently linked to this bug.
              </Typography>
              <Typography sx={{ opacity: '50%' }}>
                To link an alert, click the
                <IconButton onClick={(e) => e.stopPropagation()}>
                  <BugReportIcon />
                </IconButton>
                icon on the right of the alert.
              </Typography>
            </>
          )}
        </AccordionDetails>
      </Accordion>
    </>
  );
};
