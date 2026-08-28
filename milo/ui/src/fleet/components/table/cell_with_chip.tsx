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

import { MRT_RowData } from 'material-react-table';
import React from 'react';

import { androidState } from '@/fleet/pages/device_list_page/android/android_state';
import { swarmingState } from '@/fleet/pages/device_list_page/browser/swarming_state';
import { dutState } from '@/fleet/pages/device_list_page/chromeos/dut_state';
import { FC_CellProps } from '@/fleet/types/table';
import {
  useGoogleAnalytics,
  EventPayload,
} from '@/generic_libs/components/google_analytics';

import { ChipComponent } from './chip_component';

export type StateUnion = dutState | androidState | swarmingState;

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
  const color = getColor(valueStr as Exclude<StateUnion, ''>);
  const { trackEvent } = useGoogleAnalytics();

  return (
    <ChipComponent
      label={label ?? valueStr}
      url={url}
      color={color}
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
