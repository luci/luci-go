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
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import ConstructionIcon from '@mui/icons-material/Construction';
import FlakyIcon from '@mui/icons-material/Flaky';
import { Link, TableCell, TableRow } from '@mui/material';
import { DateTime } from 'luxon';

import { useAuthState } from '@/common/components/auth_state_provider';
import { Timestamp } from '@/common/components/timestamp';
import {
  GeneralState,
  Status,
  generalStateToJSON,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';

interface TreeStatusRowProps {
  status: Status;
}

// An row in the TreeStatusTable containing a single status update.
export function TreeStatusRow({ status }: TreeStatusRowProps) {
  const createTime = status.createTime
    ? DateTime.fromISO(status.createTime)
    : undefined;
  const identity = useAuthState();
  // External users cannot see teams pages, so only link it for Googlers.
  const showTeamsLink = /@google.com$/.test(identity.email || '');
  const fit = { width: '1px', whiteSpace: 'nowrap' };
  return (
    <TableRow hover>
      <TableCell sx={fit}>
        <GeneralStateDisplay state={status.generalState} />
      </TableCell>
      <TableCell>{status.message}</TableCell>
      <TableCell sx={fit}>
        <Timestamp datetime={createTime} />
      </TableCell>
      <TableCell sx={fit}>
        {showTeamsLink ? (
          <Link
            href={`https://moma.corp.google.com/person/${status.createUser}`}
            target="_blank"
            rel="noreferrer"
          >
            {status.createUser}
          </Link>
        ) : (
          status.createUser
        )}
      </TableCell>
    </TableRow>
  );
}

interface GeneralStateDisplayProps {
  state: GeneralState;
}

// Display component for a GeneralState value.
const GeneralStateDisplay = ({ state }: GeneralStateDisplayProps) => {
  const uppercase = generalStateToJSON(state);
  const text = uppercase.charAt(0) + uppercase.substring(1).toLocaleLowerCase();
  const icon =
    state === GeneralState.OPEN ? (
      <CheckCircleOutlineIcon />
    ) : state === GeneralState.CLOSED ? (
      <CancelIcon />
    ) : state === GeneralState.THROTTLED ? (
      <FlakyIcon />
    ) : state === GeneralState.MAINTENANCE ? (
      <ConstructionIcon />
    ) : (
      <></>
    );
  const color =
    state === GeneralState.OPEN
      ? 'var(--success-color)'
      : state === GeneralState.CLOSED
      ? 'var(--failure-color)'
      : state === GeneralState.THROTTLED
      ? 'var(--warning-text-color)'
      : state === GeneralState.MAINTENANCE
      ? 'var(--critical-failure-color)'
      : 'inherit';
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        color,
      }}
    >
      {icon} {text}
    </div>
  );
};
