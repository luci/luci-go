// Copyright 2025 The LUCI Authors.
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
import { GridRenderCellParams, GridValidRowModel } from '@mui/x-data-grid';
import React from 'react';

import { colors } from '@/fleet/theme/colors';

import { dutState } from '../../pages/device_list_page/chromeos/dut_state';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function renderChipCell<R extends GridValidRowModel = any>(
  linkGenerator: (value: string, props: GridRenderCellParams<R>) => string,
  getColor: (value: dutState) => string,
  label?: string,
  openInNewTab: boolean = true,
): (props: GridRenderCellParams<R>) => React.ReactElement {
  const ChipWithLink = (props: GridRenderCellParams<R>) => {
    const { value = '' } = props;
    const chipColor = getColor(value);
    const variant = chipColor === colors.transparent ? 'outlined' : 'filled';

    return (
      <Chip
        label={label ?? value ?? ''}
        variant={variant}
        sx={{ backgroundColor: chipColor }}
        href={linkGenerator(value, props)}
        target={openInNewTab ? '_blank' : '_self'}
        component="a"
        clickable
      ></Chip>
    );
  };
  return ChipWithLink;
}
