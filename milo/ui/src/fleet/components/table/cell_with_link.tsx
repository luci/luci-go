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
import { Link } from 'react-router';

import { sortLabelValues } from '@/fleet/components/device_table/dimensions';
import { DIMENSION_SEPARATOR } from '@/fleet/constants/dimension_separator';
import { FC_CellProps } from '@/fleet/types/table';
import {
  useGoogleAnalytics,
  EventPayload,
} from '@/generic_libs/components/google_analytics';

import { CellWithTooltip } from './cell_with_tooltip';

const getPathnameWithParams = () => {
  return window.location.href.toString().split(window.location.host)[1];
};

/**
 * Helper that generates a `renderCell` function based on link generation and optional value extraction configurations.
 * @param options The options object configuring the link and cell rendering.
 * @param options.linkGenerator A function that takes a value and turns it into a URL.
 * @param options.newTab Whether the link should open in a new tab. Defaults to true.
 * @param options.valueGetter An optional custom function to extract a value as a string from raw row data.
 * @returns A function that renders a cell with a tooltip and a link based on FC_CellProps.
 */
export interface RenderCellWithLinkOptions<R extends MRT_RowData> {
  linkGenerator: (value: string, rowOrProps: R) => string;
  newTab?: boolean;
  valueGetter?: (rowOrProps: R) => string;
  getTrackingEvent?: (
    value: string,
    url: string,
    rowOrProps: R,
  ) => { eventName: string; payload: EventPayload } | null;
}

interface LinkCellProps<R extends MRT_RowData>
  extends RenderCellWithLinkOptions<R> {
  cellProps: FC_CellProps<R>;
}

// eslint-disable-next-line react-refresh/only-export-components
const LinkCell = <R extends MRT_RowData>(props: LinkCellProps<R>) => {
  const {
    linkGenerator,
    newTab = true,
    valueGetter,
    getTrackingEvent,
    cellProps,
  } = props;

  const paramsOrRow = cellProps.row.original;
  const rawValue = cellProps.cell.getValue();
  const values = valueGetter
    ? [valueGetter(paramsOrRow)]
    : Array.isArray(rawValue)
      ? (rawValue as readonly string[])
      : [String(rawValue ?? '')];

  const sortedValues = sortLabelValues(values);
  const displayString = sortedValues.join(DIMENSION_SEPARATOR);
  const { trackEvent } = useGoogleAnalytics();

  return (
    <CellWithTooltip
      column={cellProps.column}
      value={sortedValues.map((item, index) => {
        const url = linkGenerator(item, paramsOrRow);
        return (
          <React.Fragment key={`${item}-${index}`}>
            {index > 0 && DIMENSION_SEPARATOR}
            <Link
              to={url}
              state={{
                navigatedFromLink: getPathnameWithParams(),
              }}
              target={newTab ? '_blank' : '_self'}
              onClick={
                getTrackingEvent
                  ? () => {
                      const tracking = getTrackingEvent(item, url, paramsOrRow);
                      if (tracking) {
                        trackEvent(tracking.eventName, tracking.payload);
                      }
                    }
                  : undefined
              }
            >
              {item}
            </Link>
          </React.Fragment>
        );
      })}
      tooltipTitle={displayString}
    />
  );
};
LinkCell.displayName = 'LinkCell';

export function renderCellWithLink<R extends MRT_RowData>(
  options: RenderCellWithLinkOptions<R>,
): (props: FC_CellProps<R>) => React.ReactElement {
  const Component = (props: FC_CellProps<R>) => (
    <LinkCell<R> {...options} cellProps={props} />
  );
  Component.displayName = 'renderCellWithLink';
  return Component;
}
