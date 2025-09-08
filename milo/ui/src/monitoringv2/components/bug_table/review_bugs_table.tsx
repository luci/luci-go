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

import {
  Box,
  CircularProgress,
  Link,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';

import { useBugs } from '@/monitoringv2/hooks/bugs';
import { AlertOrganizer } from '@/monitoringv2/util/alerts';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

import { BugRow } from './bug_row';

interface BugTableProps {
  groups: readonly AlertGroup[];
  organizer: AlertOrganizer;
  setSelectedTab: (tab: string) => void;
}

/**
 * A BugTable shows a list of bugs with the number of associated alerts.
 */
export const ReviewBugsTable = ({
  groups,
  organizer,
  setSelectedTab,
}: BugTableProps) => {
  const { otherHotlistBugs, groupBugs, bugIsLoading } = useBugs();
  const activeGroupKeys = organizer.activeAlertKeys();
  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell width="32px">Alerts</TableCell>
          <TableCell width="20px">P</TableCell>
          <TableCell>Title</TableCell>
          <TableCell width="100px">Status</TableCell>
          <TableCell width="100px">Modified By</TableCell>
          <TableCell width="100px">Modified</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {bugIsLoading && (
          <TableRow>
            <TableCell
              colSpan={4}
              sx={{ padding: '16px', textAlign: 'center' }}
            >
              <CircularProgress />
              <Box sx={{ opacity: '75%' }}>
                If loading the bugs takes more than 30 seconds, please check you
                are logged in to{' '}
                <Link
                  href="https://b.corp.google.com"
                  target="_blank"
                  rel="noreferrer"
                >
                  Buganizer
                </Link>{' '}
                and reload this page.
              </Box>
            </TableCell>
          </TableRow>
        )}
        {Object.entries(groupBugs).map(([alertGroup, bugs]) => (
          <>
            {bugs.map((bug) => (
              <BugRow
                key={bug.number}
                bug={bug}
                alertGroup={groups.find((g) => g.name === alertGroup)}
                alertKeys={activeGroupKeys[alertGroup]}
                setSelectedTab={setSelectedTab}
              />
            ))}
          </>
        ))}
        {otherHotlistBugs.map((bug) => (
          <BugRow
            key={bug.number}
            bug={bug}
            alertGroup={undefined}
            alertKeys={undefined}
            setSelectedTab={setSelectedTab}
          />
        ))}
      </TableBody>
    </Table>
  );
};
