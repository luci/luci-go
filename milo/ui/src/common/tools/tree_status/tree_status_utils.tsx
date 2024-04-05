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
import { SxProps } from '@mui/material';
import { ReactNode } from 'react';

import {
  GeneralState,
  generalStateToJSON,
} from '@/proto/go.chromium.org/luci/tree_status/proto/v1/tree_status.pb';

export const statusColor = (state: GeneralState | undefined): string => {
  return state === GeneralState.OPEN
    ? 'var(--success-color)'
    : state === GeneralState.CLOSED
    ? 'var(--failure-color)'
    : state === GeneralState.THROTTLED
    ? 'var(--warning-text-color)'
    : state === GeneralState.MAINTENANCE
    ? 'var(--critical-failure-color)'
    : 'inherit';
};

export interface StatusIconProps {
  state: GeneralState | undefined;
  sx?: SxProps;
}
export const StatusIcon = ({
  sx: iconStyle,
  state,
}: StatusIconProps): ReactNode => {
  return state === GeneralState.OPEN ? (
    <CheckCircleOutlineIcon sx={iconStyle} />
  ) : state === GeneralState.CLOSED ? (
    <CancelIcon sx={iconStyle} />
  ) : state === GeneralState.THROTTLED ? (
    <FlakyIcon sx={iconStyle} />
  ) : state === GeneralState.MAINTENANCE ? (
    <ConstructionIcon sx={iconStyle} />
  ) : (
    <></>
  );
};

export const statusText = (state: GeneralState | undefined): string => {
  if (state === undefined) {
    return '';
  }
  const uppercase = generalStateToJSON(state);
  return uppercase.charAt(0) + uppercase.substring(1).toLocaleLowerCase();
};
