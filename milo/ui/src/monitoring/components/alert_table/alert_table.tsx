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
  Tooltip,
} from '@mui/material';
import { Fragment, useState } from 'react';

import { AlertJson, TreeJson, BugId, Bug } from '@/monitoring/util/server_json';

import { AlertDetailsRow } from './alert_details';
import { BugMenu } from './bug_menu';
import { AlertSummaryRow } from './summary_row';

interface AlertTableProps {
  tree: TreeJson;
  alerts: AlertJson[];
  bug?: Bug;
  bugs: Bug[];
  alertBugs: { [alertKey: string]: BugId[] };
}

// An AlertTable shows a list of alerts.  There are usually several on the page at once.
export const AlertTable = ({
  tree,
  alerts,
  bug,
  bugs,
  alertBugs,
}: AlertTableProps) => {
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [expanded, setExpanded] = useState({} as { [alert: string]: boolean });
  const [expandAll, setExpandAll] = useState(false);
  if (!alerts) {
    return null;
  }
  const toggleExpandAll = () => {
    setExpanded(Object.fromEntries(alerts.map((a) => [a.key, !expandAll])));
    setExpandAll(!expandAll);
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
          <TableCell>Failed Builder</TableCell>
          <TableCell>Failed Step</TableCell>
          <TableCell>Failed Builds</TableCell>
          <TableCell>Blamelist</TableCell>
          <TableCell>
            <div style={{ display: 'flex' }}>
              <Tooltip title="Link bug to ALL alerts">
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
                alertBugs={alertBugs}
              />
              <Tooltip title="Snooze ALL alerts for 60 minutes">
                <IconButton>
                  <NotificationsPausedIcon />
                </IconButton>
              </Tooltip>
            </div>
          </TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {alerts.map((alert) => {
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
                      alertBugs={alertBugs}
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
