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
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import NotificationsIcon from '@mui/icons-material/Notifications';
import NotificationsPausedIcon from '@mui/icons-material/NotificationsPaused';
import { IconButton, TableCell, TableRow, Tooltip } from '@mui/material';
import { Link } from '@mui/material';
import { useState } from 'react';

import {
  AlertBuilderJson,
  AlertJson,
  TreeJson,
  BugId,
  Bug,
} from '@/monitoring/util/server_json';

import { BugMenu } from './bug_menu';

interface AlertSummaryRowProps {
  alert: AlertJson;
  builder: AlertBuilderJson;
  expanded: boolean;
  onExpand: () => void;
  tree: TreeJson;
  bugs: Bug[];
  alertBugs: { [alertKey: string]: BugId[] };
}
// An expandable row in the AlertTable containing a summary of a single alert.
export const AlertSummaryRow = ({
  alert,
  builder,
  expanded,
  onExpand,
  tree,
  bugs,
  alertBugs,
}: AlertSummaryRowProps) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);

  const step = stepRe.exec(alert.title)?.[1];
  // TODO: snoozing.
  const snoozed = false;
  const numTestFailures = alert.extension?.reason?.num_failing_tests || 0;
  const firstTestFailureName = shortTestName(
    alert.extension?.reason?.tests?.[0].test_name,
  );
  const failureCount =
    builder.first_failure_build_number == 0
      ? undefined
      : builder.latest_failure_build_number -
        builder.first_failure_build_number +
        1;

  return (
    <TableRow hover onClick={() => onExpand()} sx={{ cursor: 'pointer' }}>
      <TableCell>
        <IconButton>
          {expanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
        </IconButton>
      </TableCell>
      <TableCell>
        <Link
          href={builder.url}
          target="_blank"
          rel="noreferrer"
          onClick={(e) => e.stopPropagation()}
        >
          {builder.name}
        </Link>
      </TableCell>
      <TableCell>
        {step}
        {numTestFailures > 0 && (
          <span style={{ opacity: '0.7', marginLeft: '5px' }}>
            {firstTestFailureName}
            {numTestFailures > 1 && <> + {numTestFailures - 1} more</>}
          </span>
        )}
      </TableCell>
      <TableCell>
        {failureCount && (
          <>
            {failureCount} build{failureCount > 1 && 's'}:{' '}
          </>
        )}
        {builder.first_failure_url != '' ? (
          <Link
            href={builder.first_failure_url}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            {builder.first_failure_build_number}
          </Link>
        ) : (
          'Unknown'
        )}
        {builder.latest_failure_build_number !=
        builder.first_failure_build_number ? (
          <>
            {' - '}
            <Link
              href={builder.latest_failure_url}
              target="_blank"
              rel="noreferrer"
              onClick={(e) => e.stopPropagation()}
            >
              {builder.latest_failure_build_number}
            </Link>
          </>
        ) : null}
      </TableCell>
      <TableCell>
        {builder.first_failure_url == '' ? (
          'Unknown'
        ) : (
          <Link
            href={builder.first_failure_url + '/blamelist'}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            {builder.first_failing_rev?.commit_position &&
            builder.last_passing_rev?.commit_position ? (
              <>
                {builder.first_failing_rev?.commit_position -
                  builder.last_passing_rev?.commit_position}{' '}
                CL
                {builder.first_failing_rev?.commit_position -
                  builder.last_passing_rev?.commit_position >
                  1 && 's'}
              </>
            ) : (
              'Blamelist'
            )}
          </Link>
        )}
      </TableCell>
      <TableCell>
        <div style={{ display: 'flex' }}>
          <Tooltip title="Link bug">
            <IconButton
              onClick={(e) => {
                e.stopPropagation();
                setMenuAnchorEl(e.currentTarget);
              }}
            >
              <BugReportIcon />
            </IconButton>
          </Tooltip>
          <BugMenu
            anchorEl={menuAnchorEl}
            onClose={() => setMenuAnchorEl(null)}
            alerts={[alert]}
            tree={tree}
            bugs={bugs}
            alertBugs={alertBugs}
          />

          <Tooltip title="Snooze alert for 60 minutes">
            <IconButton onClick={(e) => e.stopPropagation()}>
              {snoozed ? <NotificationsIcon /> : <NotificationsPausedIcon />}
            </IconButton>
          </Tooltip>
        </div>
      </TableCell>
    </TableRow>
  );
};

// shortTestName applies various heuristics to try to get the best test name in less than 80 characters.
const shortTestName = (name: string | null | undefined): string | undefined => {
  if (!name) {
    return undefined;
  }
  const parts = name.split('/');
  let short = parts.pop();
  while (parts.length && short && short.length < 5) {
    short = parts.pop() + '/' + short;
  }
  if (short && short?.length <= 80) {
    return short;
  }
  return short?.slice(0, 77) + '...';
};

// stepRE extracts the step name from an alert title.
const stepRe = /Step "([^"]*)/;
