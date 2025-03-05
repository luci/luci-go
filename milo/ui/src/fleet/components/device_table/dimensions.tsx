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

import { getSwarmingStateDocLinkForLabel } from '@/fleet/constants/flops_doc_mapping';
import {
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { Cell } from './Cell';

// Constant for the the separator we use across the UI for displaying
// multiple values as a string string (ie: in one chip or one table cell)
// TODO: b/378634266 should be discussed how to show multiple values
export const DIMENSION_SEPARATOR = ', ';

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
// TODO: b/400795310 - Get rid of need to explicitly set getValue.
export const CROS_DIMENSION_OVERRIDES: Dimension[] = [
  {
    id: 'dut_state',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['dut_state']?.values[0] || '',
    renderCell: renderCellWithLink(getSwarmingStateDocLinkForLabel),
  },
  {
    id: 'label-servo_state',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['label-servo_state']?.values[0] || '',
    renderCell: renderCellWithLink(getSwarmingStateDocLinkForLabel),
  },
  {
    id: 'bluetooth_state',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['bluetooth_state']?.values[0] || '',
    renderCell: renderCellWithLink(getSwarmingStateDocLinkForLabel),
  },
  {
    id: 'label-model',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['label-model']?.values[0] || '',
    renderCell: renderCellWithLink((value) => `http://go/dlm-model/${value}`),
  },
  {
    id: 'label-board',
    getValue: (device: Device) =>
      device.deviceSpec?.labels['label-board']?.values[0] || '',
    renderCell: renderCellWithLink((value) => `http://go/dlm-board/${value}`),
  },
];

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
