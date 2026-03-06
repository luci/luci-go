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
import { GridRenderCellParams } from '@mui/x-data-grid';
import { MRT_RowData } from 'material-react-table';
import React from 'react';

import { useSettings } from '@/fleet/hooks/use_settings';
import { androidState } from '@/fleet/pages/device_list_page/android/android_state';
import { colors } from '@/fleet/theme/colors';
import { FC_CellProps } from '@/fleet/types/table';

import { dutState } from '../../pages/device_list_page/chromeos/dut_state';

export type StateUnion = dutState | androidState;

// eslint-disable-next-line react-refresh/only-export-components
function ChipComponent(props: {
  value: string;
  url: string;
  getColor: (value: StateUnion) => string;
  label?: string;
  openInNewTab?: boolean;
}) {
  const { value, url, getColor, label, openInNewTab } = props;

  const [settings, _] = useSettings();

  const density = settings?.table?.density;

  const chipColor = getColor(value as StateUnion);
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
      href={url}
      target={openInNewTab ? '_blank' : '_self'}
      rel={openInNewTab ? 'noopener noreferrer' : undefined}
      component="a"
      clickable
    />
  );
}

export function renderChipCell<R extends MRT_RowData>(
  getValueOrUrl: (
    value: string,
    rowOrProps: R | GridRenderCellParams<R>,
  ) => string,
  getColor: (value: StateUnion) => string,
  label?: string,
  openInNewTab: boolean = true,
  overrideValue?: StateUnion,
): (props: FC_CellProps<R> | GridRenderCellParams<R>) => React.ReactElement {
  const CellWithChip = (props: FC_CellProps<R> | GridRenderCellParams<R>) => {
    const isMRT = 'cell' in props;

    const valueStr = String(
      overrideValue ??
        (isMRT ? (props.cell.getValue() ?? '') : (props.value ?? '')),
    );
    const paramsOrRow = isMRT ? props.row.original : props;
    const url = getValueOrUrl(valueStr, paramsOrRow);

    return (
      <ChipComponent
        value={label ?? valueStr}
        url={url}
        getColor={() => getColor(valueStr as Exclude<StateUnion, ''>)}
        openInNewTab={openInNewTab}
      />
    );
  };
  CellWithChip.displayName = 'CellWithChip';
  return CellWithChip;
}
