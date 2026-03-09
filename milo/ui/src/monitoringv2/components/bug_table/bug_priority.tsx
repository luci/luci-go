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

import { Box, SxProps } from '@mui/material';

export interface BugPriorityProps {
  priority: number | undefined;
}
export const BugPriority = ({ priority }: BugPriorityProps) => {
  if (priority === undefined) {
    return null;
  }
  let style: SxProps = {
    width: '26px',
    height: '24px',
    textAlign: 'center',
    lineHeight: '20px',
    padding: '1px 2px',
    boxSizing: 'border-box',
    borderRadius: '4px',
    border: 'solid 2px #fff',
  };
  if (priority === 0) {
    style = {
      ...style,
      backgroundColor: '#1a73e8',
      border: 'solid 2px #1a73e8',
      color: '#fff',
    };
  }
  if (priority === 1) {
    style = { ...style, border: 'solid 2px #1a73e8', borderRadius: '4px' };
  }
  return <Box sx={style}>P{priority}</Box>;
};
