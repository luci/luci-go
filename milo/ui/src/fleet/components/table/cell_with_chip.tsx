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
import { MRT_RowData } from 'material-react-table';
import React from 'react';

import { useSettings } from '@/fleet/hooks/use_settings';
import { androidState } from '@/fleet/pages/device_list_page/android/android_state';
import { colors } from '@/fleet/theme/colors';
import { FC_CellProps } from '@/fleet/types/table';
import {
  useGoogleAnalytics,
  EventPayload,
} from '@/generic_libs/components/google_analytics';

import { swarmingState } from '../../pages/device_list_page/browser/swarming_state';
import { dutState } from '../../pages/device_list_page/chromeos/dut_state';

export type StateUnion = dutState | androidState | swarmingState;

// eslint-disable-next-line react-refresh/only-export-components
function ChipComponent(props: {
  value: string;
  url: string;
  getColor: (value: StateUnion) => string;
  label?: string;
  openInNewTab?: boolean;
  onClick?: () => void;
}) {
  const { value, url, getColor, label, openInNewTab, onClick } = props;

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
      onClick={onClick}
    />
  );
}

export interface RenderChipCellOptions<R extends MRT_RowData> {
  getValueOrUrl: (value: string, rowOrProps: R) => string;
  getColor: (value: StateUnion) => string;
  label?: string;
  openInNewTab?: boolean;
  overrideValue?: StateUnion;
  getTrackingEvent?: (
    value: string,
    url: string,
    rowOrProps: R,
  ) => { eventName: string; payload: EventPayload } | null;
}

interface ChipCellProps<R extends MRT_RowData>
  extends RenderChipCellOptions<R> {
  cellProps: FC_CellProps<R>;
}

// eslint-disable-next-line react-refresh/only-export-components
const ChipCell = <R extends MRT_RowData>(props: ChipCellProps<R>) => {
  const {
    getValueOrUrl,
    getColor,
    label,
    openInNewTab = true,
    overrideValue,
    getTrackingEvent,
    cellProps,
  } = props;

  const valueStr = String(overrideValue ?? cellProps.cell.getValue() ?? '');
  const paramsOrRow = cellProps.row.original;
  const url = getValueOrUrl(valueStr, paramsOrRow);
  const { trackEvent } = useGoogleAnalytics();

  return (
    <ChipComponent
      value={label ?? valueStr}
      url={url}
      getColor={() => getColor(valueStr as Exclude<StateUnion, ''>)}
      openInNewTab={openInNewTab}
      onClick={
        getTrackingEvent
          ? () => {
              const tracking = getTrackingEvent(valueStr, url, paramsOrRow);
              if (tracking) {
                trackEvent(tracking.eventName, tracking.payload);
              }
            }
          : undefined
      }
    />
  );
};
ChipCell.displayName = 'ChipCell';

export function renderChipCell<R extends MRT_RowData>(
  options: RenderChipCellOptions<R>,
): (props: FC_CellProps<R>) => React.ReactElement {
  const Component = (props: FC_CellProps<R>) => (
    <ChipCell<R> {...options} cellProps={props} />
  );
  Component.displayName = 'renderChipCell';
  return Component;
}
