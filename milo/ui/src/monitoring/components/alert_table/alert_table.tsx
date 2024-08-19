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
import NotificationsPausedIcon from '@mui/icons-material/NotificationsPaused';
import {
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TableSortLabel,
  Tooltip,
} from '@mui/material';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Fragment, useState } from 'react';

import { usePrpcServiceClient } from '@/common/hooks/prpc_query';
import { unwrapOrElse } from '@/generic_libs/tools/utils';
import { AlertJson, TreeJson, Bug } from '@/monitoring/util/server_json';
import {
  AlertsClientImpl,
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

import { AlertDetailsRow } from './alert_details';
import { BugMenu } from './bug_menu';
import { AlertSummaryRow } from './summary_row';

interface AlertTableProps {
  tree: TreeJson;
  alerts: AlertJson[];
  bug?: Bug;
  bugs: Bug[];
}

// An AlertTable shows a list of alerts.  There are usually several on the page at once.
export const AlertTable = ({ tree, alerts, bug, bugs }: AlertTableProps) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [expanded, setExpanded] = useState({} as { [alert: string]: boolean });
  const [expandAll, setExpandAll] = useState(false);
  const [sortColumn, setSortColumn] = useState<string | undefined>(undefined);
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');

  const latestBuild = (alert: AlertJson): number | undefined => {
    return alert.extension.builders?.[0].latest_failure_build_number;
  };
  const queryClient = useQueryClient();
  const client = usePrpcServiceClient({
    host: SETTINGS.luciNotify.host,
    ClientImpl: AlertsClientImpl,
  });
  const silenceAllMutation = useMutation({
    mutationFn: (alerts: AlertJson[]) => {
      // eslint-disable-next-line new-cap
      return client.BatchUpdateAlerts(
        BatchUpdateAlertsRequest.fromPartial({
          requests: alerts.map((a) =>
            UpdateAlertRequest.fromPartial({
              alert: {
                name: `alerts/${encodeURIComponent(a.key)}`,
                bug: a.bug || '0',
                silenceUntil: `${latestBuild(a) || 0}`,
              },
            }),
          ),
        }),
      );
    },
    onSuccess: () => queryClient.invalidateQueries(),
  });
  if (!alerts) {
    return null;
  }
  const toggleExpandAll = () => {
    setExpanded(Object.fromEntries(alerts.map((a) => [a.key, !expandAll])));
    setExpandAll(!expandAll);
  };
  const isSilenced = (alert: AlertJson): boolean => {
    const silenceUntil = unwrapOrElse(
      () => parseInt(alert.silenceUntil || '0'),
      () => 0,
    );
    return (latestBuild(alert) || 1) <= silenceUntil;
  };
  const sortAlerts = (alerts: AlertJson[]): AlertJson[] => {
    if (!sortColumn) {
      return alerts;
    }
    return alerts.sort((a, b) => {
      const aValue = sortValue(a, sortColumn);
      const bValue = sortValue(b, sortColumn);
      if (sortDirection === 'desc') {
        return aValue < bValue ? 1 : aValue > bValue ? -1 : 0;
      } else {
        return aValue < bValue ? -1 : aValue > bValue ? 1 : 0;
      }
    });
  };
  const sortedAlerts = [
    ...sortAlerts(alerts.filter((a) => !isSilenced(a))),
    ...sortAlerts(alerts.filter(isSilenced)),
  ];

  const clickSortColumn = (column: string) => {
    if (sortColumn === column) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortColumn(column);
      setSortDirection('asc');
    }
  };

  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell>
            <IconButton onClick={toggleExpandAll}>
              {expandAll ? <ExpandMoreIcon /> : <ChevronRightIcon />}
            </IconButton>
          </TableCell>
          <TableCell>
            <TableSortLabel
              active={sortColumn === 'failed_builder'}
              onClick={() => clickSortColumn('failed_builder')}
              direction={sortDirection}
            >
              Failed Builder
            </TableSortLabel>
          </TableCell>
          <TableCell>
            <TableSortLabel
              active={sortColumn === 'failed_step'}
              onClick={() => clickSortColumn('failed_step')}
              direction={sortDirection}
            >
              Failed Step
            </TableSortLabel>
          </TableCell>
          <TableCell>
            <TableSortLabel
              active={sortColumn === 'failed_builds'}
              onClick={() => clickSortColumn('failed_builds')}
              direction={sortDirection}
            >
              Failed Builds
            </TableSortLabel>
          </TableCell>
          <TableCell>
            <TableSortLabel
              active={sortColumn === 'blamelist'}
              onClick={() => clickSortColumn('blamelist')}
              direction={sortDirection}
            >
              Blamelist
            </TableSortLabel>
          </TableCell>
          <TableCell>
            <div style={{ display: 'flex' }}>
              <Tooltip title="Link bug to all displayed alerts">
                <IconButton onClick={(e) => setMenuAnchorEl(e.currentTarget)}>
                  <BugReportIcon />
                </IconButton>
              </Tooltip>
              <BugMenu
                anchorEl={menuAnchorEl}
                onClose={() => setMenuAnchorEl(null)}
                alerts={alerts}
                tree={tree}
                bugs={bugs}
              />
              <Tooltip title="Snooze all displayed alerts until the next build">
                <IconButton onClick={() => silenceAllMutation.mutate(alerts)}>
                  <NotificationsPausedIcon />
                </IconButton>
              </Tooltip>
            </div>
          </TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {sortedAlerts.map((alert) => {
          // There should only be one builder, but we iterate the builders just in case.
          // It will result in some UI weirdness if there are ever more than one builder, but better
          // than not showing data.
          return (
            <Fragment key={alert.key}>
              {alert.extension.builders.map((builder) => {
                return (
                  <Fragment key={builder.name}>
                    <AlertSummaryRow
                      alert={alert}
                      builder={builder}
                      expanded={expanded[alert.key]}
                      onExpand={() => {
                        const copy = { ...expanded };
                        copy[alert.key] = !copy[alert.key];
                        setExpanded(copy);
                      }}
                      tree={tree}
                      bugs={bugs}
                    />
                    {expanded[alert.key] && (
                      <AlertDetailsRow
                        tree={tree}
                        alert={alert}
                        bug={bug}
                        key={alert.key + builder.name}
                      />
                    )}
                  </Fragment>
                );
              })}
            </Fragment>
          );
        })}
      </TableBody>
    </Table>
  );
};

const sortValue = (alert: AlertJson, sortColumn: string): number | string => {
  const builder = alert.extension.builders?.[0];
  switch (sortColumn) {
    case 'failed_builder':
      return builder?.name || '';
    case 'failed_step':
      return stepRe.exec(alert.title)?.[1] || '';
    case 'failed_builds':
      return builder.first_failure_build_number === 0
        ? -1
        : builder.latest_failure_build_number -
            builder.first_failure_build_number +
            1;
    case 'blamelist':
      if (
        builder.first_failing_rev?.commit_position &&
        builder.last_passing_rev?.commit_position
      ) {
        return (
          builder.first_failing_rev?.commit_position -
          builder.last_passing_rev?.commit_position
        );
      } else {
        return -1;
      }
    default:
      return 0;
  }
};

// stepRE extracts the step name from an alert title.
const stepRe = /Step "([^"]*)/;
