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

import { Icon, SxProps, Theme } from '@mui/material';
import { IconOwnProps } from '@mui/material';

import { BUILD_STATUS_CLASS_MAP } from '@/build/constants';
import { SpecifiedBuildStatus } from '@/build/types';
import { Status } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

const BUILD_STATUS_ICON_MAP = Object.freeze({
  [Status.SCHEDULED]: 'schedule',
  [Status.STARTED]: 'pending',
  [Status.SUCCESS]: 'check_circle',
  [Status.FAILURE]: 'error',
  [Status.INFRA_FAILURE]: 'report',
  [Status.CANCELED]: 'cancel',
});

export interface BuildStatusIconProps extends IconOwnProps {
  readonly status: SpecifiedBuildStatus;
  readonly sx?: SxProps<Theme>;
}

export function BuildStatusIcon({
  status,
  sx,
  ...props
}: BuildStatusIconProps) {
  return (
    <Icon
      className={BUILD_STATUS_CLASS_MAP[status]}
      sx={{ verticalAlign: 'middle', ...sx }}
      {...props}
    >
      {BUILD_STATUS_ICON_MAP[status]}
    </Icon>
  );
}
