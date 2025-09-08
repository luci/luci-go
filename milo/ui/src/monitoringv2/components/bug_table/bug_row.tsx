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

import { Chip, Link, TableCell, TableRow, Tooltip } from '@mui/material';

import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuration } from '@/common/tools/time_utils';
import { Bug } from '@/monitoringv2/util/server_json';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

import { BugPriority } from './bug_priority';

interface BugRowProps {
  bug: Bug;
  alertGroup: AlertGroup | undefined;
  alertKeys: readonly string[] | undefined;
  setSelectedTab: (tab: string) => void;
}

/** An expandable row in the AlertTable containing a summary of a single alert. */
export const BugRow = ({
  bug,
  alertGroup,
  alertKeys,
  setSelectedTab,
}: BugRowProps) => {
  return (
    <TableRow hover>
      <TableCell width="32px">
        {alertKeys === undefined && (
          <Tooltip title="This is a hotlist bug, it is not associated with a group">
            <Chip label="H" variant="filter" />
          </Tooltip>
        )}
        {alertKeys !== undefined && (
          <Tooltip title={`Associated with group: ${alertGroup?.displayName}`}>
            <Chip
              label={alertKeys?.length || 0}
              variant="outlined"
              onClick={() => setSelectedTab('group:' + alertGroup?.name)}
            />
          </Tooltip>
        )}
      </TableCell>
      <TableCell width="20px">
        <BugPriority priority={bug.priority} />
      </TableCell>
      <TableCell>
        <Link href={bug.link} target="_blank">
          {bug.summary}
        </Link>
      </TableCell>
      <TableCell width="100px">{bug.status}</TableCell>
      <TableCell width="100px">{bug.lastModifier}</TableCell>
      <TableCell width="100px">
        <RelativeTimestamp
          timestamp={bug.modifiedTime}
          formatFn={displayApproxDuration}
        />
      </TableCell>
    </TableRow>
  );
};
