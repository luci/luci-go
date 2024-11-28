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

import { useNotifyAlertsClient } from '@/monitoringv2/hooks/prpc_clients';
import { GenericAlert } from '@/monitoringv2/pages/monitoring_page/context/context';
import { TreeJson, Bug } from '@/monitoringv2/util/server_json';
import {
  BatchUpdateAlertsRequest,
  UpdateAlertRequest,
} from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alerts.pb';

import { StructuredAlert } from '../alerts/alert_tabs';

import { BugMenu } from './bug_menu';
import { BuildAlertRow } from './build_alert_row';
import { TestAlertRow } from './test_alert_row';

interface AlertTableProps {
  tree: TreeJson;
  alerts: StructuredAlert[];
  bug?: Bug;
  bugs: Bug[];
}

type SortColumn = 'failure' | 'history';

/**
 * An AlertTable shows a list of alerts.  There are usually several on the page at once.
 */
export const AlertTable = ({ tree, alerts, bugs }: AlertTableProps) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [expanded, setExpanded] = useState({} as { [alert: string]: boolean });
  const [expandAll, setExpandAll] = useState(false);
  const [sortColumn, setSortColumn] = useState<SortColumn | undefined>(
    undefined,
  );
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');

  const queryClient = useQueryClient();
  const client = useNotifyAlertsClient();
  const silenceAllMutation = useMutation({
    mutationFn: (alerts: StructuredAlert[]) => {
      /* FIXME! do step and test alerts too. */
      // eslint-disable-next-line new-cap
      return client.BatchUpdateAlerts(
        BatchUpdateAlertsRequest.fromPartial({
          requests: alerts.map((a) =>
            UpdateAlertRequest.fromPartial({
              alert: {
                name: `alerts/${encodeURIComponent(a.alert.key)}`,
                // FIXME!
                bug: '0', // a.bug || '0',
                silenceUntil: `${a.alert.history[0].buildId || 0}`,
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
    setExpanded(
      Object.fromEntries(alerts.map((a) => [a.alert.key, !expandAll])),
    );
    setExpandAll(!expandAll);
  };
  const isSilenced = (_alert: StructuredAlert): boolean => {
    // FIXME!
    return false; // (latestBuild(alert) || '') === alert.silenceUntil;
  };
  const sortAlerts = (alerts: StructuredAlert[]): StructuredAlert[] => {
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
    ...sortAlerts(alerts.filter((a) => isSilenced(a))),
  ];

  const clickSortColumn = (column: SortColumn) => {
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
              active={sortColumn === 'failure'}
              onClick={() => clickSortColumn('failure')}
              direction={sortDirection}
            >
              Failure
            </TableSortLabel>
          </TableCell>
          <TableCell>
            <TableSortLabel
              active={sortColumn === 'history'}
              onClick={() => clickSortColumn('history')}
              direction={sortDirection}
            >
              History
            </TableSortLabel>
          </TableCell>
          <TableCell>First Failure</TableCell>
          <TableCell>Blamelist</TableCell>
          <TableCell>
            <div css={{ display: 'flex' }}>
              <Tooltip title="Link bug to all displayed alerts">
                <IconButton onClick={(e) => setMenuAnchorEl(e.currentTarget)}>
                  <BugReportIcon />
                </IconButton>
              </Tooltip>
              <BugMenu
                anchorEl={menuAnchorEl}
                onClose={() => setMenuAnchorEl(null)}
                alerts={
                  alerts.map(
                    (a) => a.alert.key,
                  ) /* FIXME: need step and test alerts too */
                }
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
        <AlertRows
          alerts={sortedAlerts}
          indent={0}
          bugs={bugs}
          tree={tree}
          expanded={expanded}
          setExpanded={setExpanded}
        />
      </TableBody>
    </Table>
  );
};

interface AlertRowsProps {
  alerts: StructuredAlert[];
  parentAlert?: GenericAlert;
  indent: number;
  bugs: Bug[];
  tree: TreeJson;
  expanded: { [alertPath: string]: boolean };
  setExpanded: (value: { [alertPath: string]: boolean }) => void;
}
const AlertRows = ({
  alerts,
  parentAlert,
  indent,
  bugs,
  tree,
  expanded,
  setExpanded,
}: AlertRowsProps) => {
  return (
    <>
      {alerts.map((alert) => {
        const path = alert.alert.key;
        return (
          <Fragment key={path}>
            {alert.alert.kind === 'test' ? (
              <TestAlertRow
                bugs={bugs}
                tree={tree}
                alert={alert}
                expanded={expanded[path]}
                onExpand={() =>
                  setExpanded({ ...expanded, [path]: !expanded[path] })
                }
                parentAlert={parentAlert}
                indent={indent}
              />
            ) : (
              <BuildAlertRow
                alert={alert}
                parentAlert={parentAlert}
                expanded={expanded[path]}
                onExpand={() =>
                  setExpanded({ ...expanded, [path]: !expanded[path] })
                }
                indent={indent}
                tree={tree}
                bugs={bugs}
              />
            )}
            {expanded[path] && (
              <AlertRows
                alerts={alert.children}
                parentAlert={alert.alert}
                indent={indent + 1}
                bugs={bugs}
                tree={tree}
                expanded={expanded}
                setExpanded={setExpanded}
              />
            )}
          </Fragment>
        );
      })}
    </>
  );
};

const sortValue = (a: StructuredAlert, column: SortColumn): string | number => {
  switch (column) {
    case 'failure':
      return a.alert.key;
    case 'history':
      return a.alert.consecutiveFailures;
    default:
      throw new Error(`Unknown sort column: ${column}`);
  }
};
