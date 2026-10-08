// Copyright 2026 The LUCI Authors.
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

import { Box, Button, Tooltip } from '@mui/material';

import { UserAvatar } from './user_avatar';

export interface AssigneeCellProps {
  readonly claimedBy?: string;
  readonly taskId: string;
  readonly currentUser: string;
  readonly isPending: boolean;
  readonly claimTask: (req: { readonly taskId: string }) => void;
  readonly unclaimTask: (req: { readonly taskId: string }) => void;
}

/**
 * Renders the assignee of a repair task: the claimer's avatar (click to
 * unclaim or steal the task) or a "Claim" button when the task is unclaimed.
 */
export const AssigneeCell = ({
  claimedBy = '',
  taskId,
  currentUser,
  isPending,
  claimTask,
  unclaimTask,
}: AssigneeCellProps) => {
  const trimmedClaimedBy = claimedBy.trim();

  if (trimmedClaimedBy) {
    const displayClaimedBy = trimmedClaimedBy.replace(/^user:/, '');
    const isSelf = Boolean(
      currentUser &&
        (trimmedClaimedBy === currentUser ||
          (currentUser.startsWith('user:') &&
            trimmedClaimedBy === currentUser.replace(/^user:/, '')) ||
          (trimmedClaimedBy.startsWith('user:') &&
            trimmedClaimedBy.replace(/^user:/, '') === currentUser)),
    );

    const tooltipTitle = isSelf
      ? 'Assigned to you (click to unclaim)'
      : `Assigned to ${displayClaimedBy} (click to assign to yourself)`;

    const handleClick = () => {
      if (isPending) return;
      if (isSelf) {
        unclaimTask({ taskId });
      } else {
        claimTask({ taskId });
      }
    };

    return (
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'center',
          width: '100%',
        }}
      >
        <Tooltip title={tooltipTitle}>
          <UserAvatar
            email={displayClaimedBy}
            onClick={handleClick}
            sx={{
              width: 26,
              height: 26,
              fontSize: '0.85rem',
              cursor: isPending ? 'not-allowed' : 'pointer',
              opacity: isPending ? 0.6 : 1,
              pointerEvents: isPending ? 'none' : 'auto',
              '&:hover': {
                opacity: isPending ? 0.6 : 0.8,
              },
            }}
          />
        </Tooltip>
      </Box>
    );
  }

  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', width: '100%' }}>
      <Button
        variant="outlined"
        size="small"
        disabled={isPending}
        sx={{
          borderRadius: '16px',
          textTransform: 'none',
          minWidth: '64px',
          height: '26px',
        }}
        onClick={() => claimTask({ taskId })}
      >
        Claim
      </Button>
    </Box>
  );
};
