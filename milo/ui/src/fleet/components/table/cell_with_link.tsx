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

import { GridRenderCellParams, GridValidRowModel } from '@mui/x-data-grid';
import React from 'react';
import { Link } from 'react-router';

import { DIMENSION_SEPARATOR } from '@/fleet/constants/dimension_separator';

import { CellWithTooltip } from './cell_with_tooltip';

const getPathnameWithParams = () => {
  return window.location.href.toString().split(window.location.host)[1];
};

/**
 * Helper that generates a `renderCell` function based on a link generator.
 * @param linkGenerator A function that takes a value and turns it into a URL.
 * @returns A function that renders a <DeviceDataCell /> based on GridRenderCellParams
 */
// TODO: b/394202288 - Add tests for this function.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function renderCellWithLink<R extends GridValidRowModel = any>(
  linkGenerator: (value: string, props: GridRenderCellParams<R>) => string,
  newTab: boolean = true,
): (props: GridRenderCellParams) => React.ReactElement {
  const CellWithLink = (props: GridRenderCellParams) => {
    const { value = '' } = props;
    const links: string[] = value.split(DIMENSION_SEPARATOR);

    return (
      <CellWithTooltip
        {...props}
        value={links.map((v: string, i: number) => (
          <React.Fragment key={v}>
            <Link
              key={v}
              to={linkGenerator(v, props)}
              state={{
                navigatedFromLink: getPathnameWithParams(),
              }}
              target={newTab ? '_blank' : '_self'}
            >
              {v}
            </Link>
            {i < links.length - 1 ? DIMENSION_SEPARATOR : ''}
          </React.Fragment>
        ))}
        tooltipTitle={value}
      />
    );
  };
  return CellWithLink;
}
