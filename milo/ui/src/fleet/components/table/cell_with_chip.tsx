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
import {
  GridRenderCellParams,
  GridValidRowModel,
  useGridApiContext,
  useGridSelector,
  gridDensitySelector,
} from '@mui/x-data-grid';
import React from 'react';

import { colors } from '@/fleet/theme/colors';

import { dutState } from '../../pages/device_list_page/chromeos/dut_state';

function ChipComponent<R extends GridValidRowModel>(
  props: GridRenderCellParams<R> & {
    linkGenerator: (value: string, props: GridRenderCellParams<R>) => string;
    getColor: (value: dutState) => string;
    label?: string;
    openInNewTab?: boolean;
  },
) {
  const { value = '', linkGenerator, getColor, label, openInNewTab } = props;
  const apiRef = useGridApiContext();
  const density = useGridSelector(apiRef, gridDensitySelector);

  const chipColor = getColor(value);
  const variant = chipColor === colors.transparent ? 'outlined' : 'filled';

  return (
    <Chip
      label={label ?? value ?? ''}
      variant={variant}
      size={density === 'compact' ? 'small' : 'medium'}
      sx={{
        backgroundColor: chipColor,
        width: 'fit-content',
        fontWeight: 500,
      }}
      href={linkGenerator(value, props)}
      target={openInNewTab ? '_blank' : '_self'}
      rel={openInNewTab ? 'noopener noreferrer' : undefined}
      component="a"
      clickable
    />
  );
}

export function renderChipCell<R extends GridValidRowModel = GridValidRowModel>(
  linkGenerator: (value: string, props: GridRenderCellParams<R>) => string,
  getColor: (value: dutState) => string,
  label?: string,
  openInNewTab: boolean = true,
): (props: GridRenderCellParams<R>) => React.ReactElement {
  return function ChipCell(props: GridRenderCellParams<R>) {
    return (
      <ChipComponent
        {...props}
        linkGenerator={linkGenerator}
        getColor={getColor}
        label={label}
        openInNewTab={openInNewTab}
      />
    );
  };
}
