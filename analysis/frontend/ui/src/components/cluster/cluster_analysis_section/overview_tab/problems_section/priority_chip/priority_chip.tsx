// Copyright 2023 The LUCI Authors.
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

import Chip from '@mui/material/Chip';
import Typography from '@mui/material/Typography';

import { BuganizerPriority, buganizerPriorityToJSON } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';

interface Props {
  priority: BuganizerPriority;
}

export const PriorityChip = ({ priority }: Props) => {
  return <Chip
    color={(priority == BuganizerPriority.P0 || BuganizerPriority.P1) ? 'primary' : 'default'}
    variant={priority == BuganizerPriority.P0 ? 'filled' : 'outlined'}
    size="medium"
    data-testid="problem_priority_chip"
    label={
      <Typography
        sx={{ fontWeight: (priority == BuganizerPriority.P0 ? 'bold' : 'regular') }}
        color="inherit">
        {/* Returns string representation of enum value, e.g. 'P2'. */}
        {buganizerPriorityToJSON(priority)}
      </Typography>
    } />;
};
