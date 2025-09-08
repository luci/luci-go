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

import CancelIcon from '@mui/icons-material/Cancel';
import {
  Box,
  CircularProgress,
  IconButton,
  Link,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
} from '@mui/material';

import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuration } from '@/common/tools/time_utils';
import { useAlertGroups } from '@/monitoringv2/hooks/alert_groups';
import { useBugs } from '@/monitoringv2/hooks/bugs';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

import { BugPriority } from './bug_priority';

interface BugTableProps {
  group: AlertGroup;
}

/**
 * A GroupBugTable shows a list of bugs associated with a single group.
 */
export const GroupBugTable = ({ group }: BugTableProps) => {
  const { update: updateGroup } = useAlertGroups();
  const { groupBugs, bugIsLoading } = useBugs();
  const bugs = groupBugs[group.name];
  return (
    <Table size="small">
      <TableHead>
        <TableRow>
          <TableCell>Bug</TableCell>
          <TableCell width="20px"></TableCell>
          <TableCell width="60px">Status</TableCell>
          <TableCell width="100px">Modified By</TableCell>
          <TableCell width="100px">Modified</TableCell>
          <TableCell width="32px"></TableCell>
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
                {/* TODO: Is it possible to detect the problem with the buganizer requests? */}
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
        {bugs.map((bug) => (
          <TableRow hover key={bug.number}>
            <TableCell>
              <Link href={bug.link} target="_blank">
                {bug.summary}
              </Link>
            </TableCell>
            <TableCell width="20px">
              <BugPriority priority={bug.priority} />
            </TableCell>
            <TableCell width="60px">{bug.status}</TableCell>
            <TableCell width="100px">{bug.lastModifier}</TableCell>
            <TableCell width="100px">
              <RelativeTimestamp
                timestamp={bug.modifiedTime}
                formatFn={displayApproxDuration}
              />
            </TableCell>
            <TableCell width="32px">
              <Tooltip title="Unlink bug from group">
                <IconButton
                  color="inherit"
                  size="small"
                  onClick={() => {
                    updateGroup.mutate({
                      alertGroup: {
                        ...group,
                        bugs: group.bugs.filter((b) => b !== bug.number),
                      },
                      updateMask: ['bugs'],
                    });
                  }}
                >
                  <CancelIcon />
                </IconButton>
              </Tooltip>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
};
