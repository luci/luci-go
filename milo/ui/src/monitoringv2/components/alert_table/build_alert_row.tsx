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
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useNotifyAlertsClient } from '@/monitoringv2/hooks/prpc_clients';
import { GenericAlert } from '@/monitoringv2/pages/monitoring_page/context/context';
import { TreeJson, Bug } from '@/monitoringv2/util/server_json';
import {
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

import { StructuredAlert } from '../alerts/alert_tabs';
import { BuilderHistorySparkline } from '../builder_history_sparkline';

import { BugMenu } from './bug_menu';
import { PrefillFilterIcon } from './prefill_filter_icon';

interface BuildAlertRowProps {
  alert: StructuredAlert;
  parentAlert?: GenericAlert;
  expanded: boolean;
  indent: number;
  onExpand: () => void;
  tree: TreeJson;
  bugs: Bug[];
}

// An expandable row in the AlertTable containing a summary of a single alert.
export const BuildAlertRow = ({
  parentAlert,
  alert,
  expanded,
  onExpand,
  indent,
  tree,
  bugs,
}: BuildAlertRowProps) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);

  const queryClient = useQueryClient();
  const client = useNotifyAlertsClient();
  const silenceMutation = useMutation({
    mutationFn: (alertToSilence: GenericAlert | null) => {
      // eslint-disable-next-line new-cap
      return client.BatchUpdateAlerts(
        BatchUpdateAlertsRequest.fromPartial({
          requests: [
            UpdateAlertRequest.fromPartial({
              alert: {
                name: `alerts/${encodeURIComponent(buildAlert.key)}`,
                // FIXME!
                bug: '0', // alert.bug || '0',
                silenceUntil: alertToSilence
                  ? buildAlert.history[0].buildId
                  : '0',
              },
            }),
          ],
        }),
      );
    },
    onSuccess: () => queryClient.invalidateQueries(),
  });
  const buildAlert = alert.alert;
  // FIXME!
  const silenced = false;
  // buildIdFromUrl(builder.latest_failure_url) === alert.silenceUntil;
  // const numTestFailures = alert.extension?.reason?.num_failing_tests || 0;
  // const firstTestFailureName = shortTestName(
  //   alert.extension?.reason?.tests?.[0].test_name,
  // );
  const id = buildAlert.builderID;
  const consecutiveFailures = buildAlert.consecutiveFailures;
  const firstFailureId = buildAlert.history[consecutiveFailures - 1]?.buildId;

  if (buildAlert.kind !== 'builder' && buildAlert.kind !== 'step') {
    throw new Error(
      `StepAlertRow can only display builder and step alerts, not ${buildAlert.kind}`,
    );
  }
  return (
    <TableRow
      hover
      onClick={() => onExpand()}
      sx={{
        cursor: 'pointer',
        opacity:
          silenced ||
          consecutiveFailures === 0 ||
          (parentAlert &&
            buildAlert.consecutiveFailures > parentAlert.consecutiveFailures)
            ? '0.5'
            : '1',
      }}
    >
      <TableCell>
        {alert.children.length > 0 && (
          <IconButton sx={{ marginLeft: `${indent * 20}px` }}>
            {expanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
          </IconButton>
        )}
      </TableCell>
      <TableCell>
        <span>
          <span
            style={{
              paddingLeft: `${indent * 20}px`,
              opacity: '75%',
              fontSize: '90%',
              fontWeight: 300,
            }}
          >
            {buildAlert.kind === 'step' ? 'Step:' : 'Builder:'}
          </span>{' '}
          {buildAlert.kind === 'step'
            ? buildAlert.stepName
            : buildAlert.builderID.builder}
          <PrefillFilterIcon filter={id.builder} />
          {!expanded && alert.children.length > 0 && (
            <span style={{ opacity: '50%' }}>
              {' '}
              {shortName(alert.children[0].alert)}
            </span>
          )}
          {!expanded && alert.children.length > 1 && (
            <span style={{ opacity: '50%' }}>
              {' '}
              + {alert.children.length - 1} more
            </span>
          )}
        </span>
      </TableCell>
      <TableCell>
        <BuilderHistorySparkline
          builderId={id}
          history={buildAlert.history}
          numHighlighted={consecutiveFailures}
        />
      </TableCell>
      <TableCell>
        {consecutiveFailures > 0 && (
          <Link
            href={`/b/${firstFailureId}`}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            {consecutiveFailures} build
            {consecutiveFailures > 1 && 's'} ago
          </Link>
        )}
      </TableCell>
      <TableCell>
        {firstFailureId && (
          <Link
            href={`/b/${firstFailureId}/blamelist`}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
          >
            {/* FIXME! {builder.first_failing_rev?.commit_position &&
          builder.last_passing_rev?.commit_position ? (
            <>
            {builder.first_failing_rev?.commit_position -
            builder.last_passing_rev?.commit_position}{' '}
            CL
            {builder.first_failing_rev?.commit_position -
            builder.last_passing_rev?.commit_position >
            1 && 's'}
            </>
            ) : ( */}
            Blamelist
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
            alerts={[buildAlert.key]}
            tree={tree}
            bugs={bugs}
          />

          <Tooltip title="Silence alert until next build completes">
            <IconButton
              onClick={(e) => {
                e.stopPropagation();
                silenceMutation.mutate(silenced ? null : buildAlert);
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

/** shortName applies various heuristics to try to get the best test/step/builder name in less than 80 characters. */
const shortName = (alert: GenericAlert): string | undefined => {
  const name =
    alert.kind === 'test'
      ? alert.testId
      : alert.kind === 'step'
        ? alert.stepName
        : alert.builderID.builder;
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
