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

import { GridRenderCellParams } from '@mui/x-data-grid';
import React from 'react';
import { Link } from 'react-router-dom';

import { CROS_DIMENSION_DOC_MAPPING } from '@/fleet/config/device_config';
import {
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { Cell } from './Cell';

// Constant for the the separator we use across the UI for displaying
// multiple values as a string string (ie: in one chip or one table cell)
// TODO: b/378634266 should be discussed how to show multiple values
export const DIMENSION_SEPARATOR = ', ';
export const labelValuesToString = (labels: readonly string[]): string => {
  return labels
    .concat()
    .sort((a, b) => (a.length < b.length ? 1 : -1))
    .join(DIMENSION_SEPARATOR);
};

interface Dimension {
  id: string; // unique id used for sorting and filtering
  displayName?: string;
  getValue: (device: Device) => string;
  renderCell?: (props: GridRenderCellParams) => React.JSX.Element;
}

const getPathnameWithParams = () => {
  return window.location.href.toString().split(window.location.host)[1];
};

/**
 * Helper that generates a `renderCell` function based on a link generator.
 * @param linkGenerator A function that takes a value and turns it into a URL.
 * @returns A function that renders a <Cell /> based on GridRenderCellParams
 */
// TODO: b/394202288 - Add tests for this function.
function renderCellWithLink(
  linkGenerator: (value: string) => string,
  newTab: boolean = true,
): (props: GridRenderCellParams) => React.ReactElement {
  const CellWithLink = (props: GridRenderCellParams) => {
    const { value = '' } = props;

    const links = value.split(DIMENSION_SEPARATOR).map((v: string) => (
      <Link
        key={v}
        to={linkGenerator(v)}
        state={{
          navigatedFromLink: getPathnameWithParams(),
        }}
        target={newTab ? '_blank' : '_self'}
      >
        {v}
      </Link>
    ));

    return (
      <Cell
        {...props}
        value={links.map((link: React.ReactElement, i: number) => (
          <>
            {link}
            {i < links.length - 1 ? DIMENSION_SEPARATOR : ''}
          </>
        ))}
        tooltipTitle={value}
      />
    );
  };
  return CellWithLink;
}

/**
 * BASE_DIMENSIONS are dimensions associated with a device that are not labels,
 * which essentially defined how the UI renders non-label fields from the
 * `ListDevices` response.
 */
export const BASE_DIMENSIONS: Dimension[] = [
  {
    id: 'id',
    displayName: 'ID',
    getValue: (device: Device) => device.id,
    renderCell: renderCellWithLink(
      (value) => `/ui/fleet/labs/devices/${value}`,
      false,
    ),
  },
  // TODO(b/400711755): Rename this to Asset Tag
  {
    id: 'dut_id',
    displayName: 'Dut ID',
    getValue: (device: Device) => device.dutId,
  },
  {
    id: 'type',
    displayName: 'Type',
    getValue: (device: Device) => DeviceType[device.type],
  },
  {
    id: 'state',
    displayName: 'Lease state',
    getValue: (device: Device) => DeviceState[device.state],
  },
  {
    id: 'host',
    displayName: 'Address',
    getValue: (device: Device) => device.address?.host || '',
  },
  {
    id: 'port',
    displayName: 'Port',
    getValue: (device: Device) => String(device.address?.port) || '',
  },
];

/**
 * Customized ChromeOS config for applying custom overrides for different
 * dimensions. Used, for example, to add doc links to specific table cells.
 */
export const CROS_DIMENSION_OVERRIDES: Dimension[] =
  CROS_DIMENSION_DOC_MAPPING.map(({ id, linkGenerator }) => ({
    id,
    // The first Array entry of values is a comma separated string with all
    // values.
    getValue: (device: Device) =>
      device.deviceSpec?.labels[id]?.values[0] || '',
    renderCell: renderCellWithLink(linkGenerator),
  }));

/**
 * Constant with all of the different configured overrides for columns. The
 * UI has default logic to handle all label data, but will apply special logic
 * in the case of certain commonly used labels (ie: labels containing
 * links to docs)
 */
export const COLUMN_OVERRIDES: Dimension[] = [
  ...BASE_DIMENSIONS,
  ...CROS_DIMENSION_OVERRIDES,
];
