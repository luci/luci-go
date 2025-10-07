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
import { Stack } from '@mui/material';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useNotifyAlertsClient } from '@/monitoring/hooks/prpc_clients';
import {
  AlertBuilderJson,
  AlertJson,
  TreeJson,
  Bug,
  buildIdFromUrl,
} from '@/monitoring/util/server_json';
import {
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

import { BuilderHistorySparkline } from '../builder_history_sparkline';

import { BugMenu } from './bug_menu';
import { PrefillFilterIcon } from './prefill_filter_icon';

interface AlertSummaryRowProps {
  alert: AlertJson;
  builder: AlertBuilderJson;
  expanded: boolean;
  onExpand: () => void;
  tree: TreeJson;
  bugs: Bug[];
}

// An expandable row in the AlertTable containing a summary of a single alert.
export const AlertSummaryRow = ({
  alert,
  builder,
  expanded,
  onExpand,
  tree,
  bugs,
}: AlertSummaryRowProps) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);

  const queryClient = useQueryClient();
  const client = useNotifyAlertsClient();
  const silenceMutation = useMutation({
    mutationFn: (builder: AlertBuilderJson | null) => {
      return client.BatchUpdateAlerts(
        BatchUpdateAlertsRequest.fromPartial({
          requests: [
            UpdateAlertRequest.fromPartial({
              alert: {
                name: `alerts/${encodeURIComponent(alert.key)}`,
                bug: alert.bug || '0',
                silenceUntil: builder
                  ? buildIdFromUrl(builder.latest_failure_url)
                  : '0',
              },
            }),
          ],
        }),
      );
    },
    onSuccess: () => queryClient.invalidateQueries(),
  });
  const step = stepRe.exec(alert.title)?.[1];
  const silenced =
    buildIdFromUrl(builder.latest_failure_url) === alert.silenceUntil;
  const numTestFailures = alert.extension?.reason?.num_failing_tests || 0;
  const firstTestFailureName = shortTestName(
    alert.extension?.reason?.tests?.[0].test_name,
  );
  const failureCount =
    builder.first_failure_build_number === 0
      ? undefined
      : builder.latest_failure_build_number -
        builder.first_failure_build_number +
        1;

  return (
    <TableRow
      hover
      onClick={() => onExpand()}
      sx={{ cursor: 'pointer', opacity: silenced ? '0.5' : '1' }}
    >
      <TableCell>
        <IconButton>
          {expanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
        </IconButton>
      </TableCell>
      <TableCell>
        <Stack alignItems="center" direction="row">
          <Link
            href={builder.url}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            {builder.name}
          </Link>
          <PrefillFilterIcon filter={builder.name} />
        </Stack>
      </TableCell>
      <TableCell>
        <BuilderHistorySparkline
          builderId={{
            project: builder.project,
            bucket: builder.bucket,
            builder: builder.name,
          }}
        />
      </TableCell>
      <TableCell>
        {step}
        {numTestFailures > 0 && (
          <span css={{ opacity: '0.7', marginLeft: '5px' }}>
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
        {builder.first_failure_url !== '' ? (
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
        {builder.latest_failure_build_number !==
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
        {builder.first_failure_url === '' ? (
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
        <div css={{ display: 'flex' }}>
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
          />

          <Tooltip title="Silence alert until next build completes">
            <IconButton
              onClick={(e) => {
                e.stopPropagation();
                silenceMutation.mutate(silenced ? null : builder);
              }}
            >
              {silenced ? <NotificationsIcon /> : <NotificationsPausedIcon />}
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
