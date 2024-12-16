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

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Checkbox,
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';
import { Fragment, useState } from 'react';

import {
  GenericAlert,
  sortAlerts,
  SortColumn,
  SortDirection,
  StructuredAlert,
} from '@/monitoringv2/util/alerts';

import { AlertGroup } from '../alerts';

import { ActionBar } from './action_bar';
import { BuildAlertRow } from './build_alert_row';
import { ColumnHeaders } from './column_headers';
import { TestAlertRow } from './test_alert_row';

interface AlertTableProps {
  alerts: StructuredAlert[];
  group?: AlertGroup;
  groups: AlertGroup[];
  setGroups: (group: AlertGroup[]) => void;
}

/**
 * An AlertTable shows a list of alerts.  There are usually several on the page at once.
 */
export const AlertTable = ({
  alerts,
  group,
  groups,
  setGroups,
}: AlertTableProps) => {
  const [expanded, setExpanded] = useState({} as { [alert: string]: boolean });
  const [expandAll, setExpandAll] = useState(false);
  const [sortColumn, setSortColumn] = useState<SortColumn | undefined>(
    undefined,
  );
  const [sortDirection, setSortDirection] = useState<SortDirection>('asc');
  const [selectedAlertKeys, setSelectedAlertKeys] = useState(
    {} as { [key: string]: boolean },
  );

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

  const sortedAlerts = [
    ...sortAlerts(
      alerts.filter((a) => !isSilenced(a)),
      sortColumn,
      sortDirection,
    ),
    ...sortAlerts(
      alerts.filter((a) => isSilenced(a)),
      sortColumn,
      sortDirection,
    ),
  ];

  const clickSortColumn = (column: SortColumn) => {
    if (sortColumn === column) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortColumn(column);
      setSortDirection('asc');
    }
  };

  const allSelected = Object.keys(selectedAlertKeys).length === alerts.length;
  const atLeastOneSelected = Object.keys(selectedAlertKeys).length > 0;
  const handleSelectAllAlerts = () => {
    if (allSelected || atLeastOneSelected) {
      setSelectedAlertKeys({});
    } else {
      setSelectedAlertKeys(
        Object.fromEntries(alerts.map((a) => [a.alert.key, true])),
      );
    }
  };
  const toggleSelectedAlertKey = (key: string) => {
    const copy = { ...selectedAlertKeys };
    if (copy[key]) {
      delete copy[key];
    } else {
      copy[key] = true;
    }
    setSelectedAlertKeys(copy);
  };

  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell width="32px" padding="none">
            <Checkbox
              checked={allSelected}
              indeterminate={!allSelected && atLeastOneSelected}
              onChange={handleSelectAllAlerts}
            />
          </TableCell>
          <TableCell width="32px" padding="none">
            <div css={{ display: 'flex' }}>
              <IconButton onClick={toggleExpandAll}>
                {expandAll ? <ExpandMoreIcon /> : <ChevronRightIcon />}
              </IconButton>
            </div>
          </TableCell>
          {atLeastOneSelected ? (
            <ActionBar
              group={group}
              groups={groups}
              setGroups={setGroups}
              alerts={alerts}
              selectedAlertKeys={selectedAlertKeys}
              unselectAll={() => setSelectedAlertKeys({})}
            />
          ) : (
            <ColumnHeaders
              sortColumn={sortColumn}
              sortDirection={sortDirection}
              clickSortColumn={clickSortColumn}
            />
          )}
        </TableRow>
      </TableHead>
      <TableBody>
        <AlertRows
          alerts={sortedAlerts}
          indent={0}
          expanded={expanded}
          setExpanded={setExpanded}
          selectedAlertKeys={selectedAlertKeys}
          toggleSelectedAlertKey={toggleSelectedAlertKey}
        />
      </TableBody>
    </Table>
  );
};

interface AlertRowsProps {
  alerts: StructuredAlert[];
  parentAlert?: GenericAlert;
  indent: number;
  expanded: { [alertPath: string]: boolean };
  setExpanded: (value: { [alertPath: string]: boolean }) => void;
  selectedAlertKeys: { [alertPath: string]: boolean };
  toggleSelectedAlertKey: (alertPath: string) => void;
}
const AlertRows = ({
  alerts,
  parentAlert,
  indent,
  expanded,
  setExpanded,
  selectedAlertKeys,
  toggleSelectedAlertKey,
}: AlertRowsProps) => {
  return (
    <>
      {alerts.map((alert) => {
        const path = alert.alert.key;
        return (
          <Fragment key={path}>
            {alert.alert.kind === 'test' ? (
              <TestAlertRow
                alert={alert}
                expanded={expanded[path]}
                onExpand={() =>
                  setExpanded({ ...expanded, [path]: !expanded[path] })
                }
                parentAlert={parentAlert}
                indent={indent}
                selected={selectedAlertKeys[path] || false}
                toggleSelected={() => toggleSelectedAlertKey(path)}
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
                selected={selectedAlertKeys[path] || false}
                toggleSelected={() => toggleSelectedAlertKey(path)}
              />
            )}
            {expanded[path] && (
              <AlertRows
                alerts={alert.children}
                parentAlert={alert.alert}
                indent={indent + 1}
                expanded={expanded}
                setExpanded={setExpanded}
                selectedAlertKeys={selectedAlertKeys}
                toggleSelectedAlertKey={toggleSelectedAlertKey}
              />
            )}
          </Fragment>
        );
      })}
    </>
  );
};
