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

import GroupIcon from '@mui/icons-material/Group';
import { Box, Button } from '@mui/material';

import { StatusPill } from '@/fleet/components/status_pill/status_pill';

export interface WorkforceActivityButtonProps {
  inProgressCount: number;
  onClick: () => void;
}

export const WorkforceActivityButton = ({
  inProgressCount,
  onClick,
}: WorkforceActivityButtonProps) => (
  <Button variant="toolbar" onClick={onClick} startIcon={<GroupIcon />}>
    Workforce Activity
    <Box component="span" sx={{ ml: 1 }}>
      <StatusPill tone="info" dot={false} size="compact">
        {`${inProgressCount} In-Progress`}
      </StatusPill>
    </Box>
  </Button>
);
