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

import DoneIcon from '@mui/icons-material/Done';
import Chip from '@mui/material/Chip';

import { BugManagementPolicy } from '@/proto/go.chromium.org/luci/analysis/proto/v1/projects.pb';

type Color = 'error' | 'default' | 'primary' | 'secondary' | 'success' | 'warning' | 'info';

const COLORS_LIST : Color[] = ['primary', 'secondary', 'warning', 'error', 'success', 'info', 'default'];

interface Props {
  policy: BugManagementPolicy;
  // Whether the chip should be shown at 50% opacity instead of 100%.
  fadedOut?: boolean;
  // Whether the problem is active.
  active?: boolean;
  // The index into the list of colors to use for this problem.
  colorIndex: number;
}

const ProblemChip = ({ policy, fadedOut, active, colorIndex } : Props ) => {
  return <Chip
    size="small"
    sx={{ margin: '1px', opacity: (fadedOut ? '50%' : undefined) }}
    label={<>{active ? (<><strong>{policy.priority}</strong>&nbsp;&nbsp;</>) : undefined}{policy.id}</>}
    color={COLORS_LIST[colorIndex % COLORS_LIST.length]}
    icon={!active ? (<DoneIcon />) : undefined}
    variant={active ? 'filled' : 'outlined'}
  />;
};

export default ProblemChip;
