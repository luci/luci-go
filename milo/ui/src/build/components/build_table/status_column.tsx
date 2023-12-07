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

import { Icon, TableCell } from '@mui/material';

import {
  BUILD_STATUS_CLASS_MAP,
  BUILD_STATUS_DISPLAY_MAP,
  BUILD_STATUS_ICON_MAP,
} from '@/build/common';

import { useBuild } from './context';

export function StatusHeadCell() {
  return (
    // Use a shorthand with a tooltip to make the column narrower.
    <TableCell width="1px" title="Status" sx={{ textAlign: 'center' }}>
      S
    </TableCell>
  );
}

export function StatusContentCell() {
  const build = useBuild();

  return (
    <TableCell>
      <Icon
        className={BUILD_STATUS_CLASS_MAP[build.status]}
        title={BUILD_STATUS_DISPLAY_MAP[build.status]}
        sx={{ transform: 'translateY(5px)' }}
      >
        {BUILD_STATUS_ICON_MAP[build.status]}
      </Icon>
    </TableCell>
  );
}
