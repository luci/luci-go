// Copyright 2026 The LUCI Authors.
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
import _ from 'lodash';
import React from 'react';

import { labelValuesToString } from '@/fleet/components/device_table/dimensions';
import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import {
  BROWSER_SWARMING_SOURCE,
  BROWSER_UFS_SOURCE,
} from '@/fleet/constants/browser';
import { GetBrowserDeviceDimensionsResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  BrowserColumnDef,
  CUSTOM_COLUMNS,
  BROWSER_COLUMN_OVERRIDES,
} from './browser_fields';

const destructureColumnId = (id: string) => {
  let source: string | undefined = undefined;

  if (id.startsWith(BROWSER_SWARMING_SOURCE)) {
    source = BROWSER_SWARMING_SOURCE;
  } else if (id.startsWith(BROWSER_UFS_SOURCE)) {
    source = BROWSER_UFS_SOURCE;
  }

  const labelKey = source ? id.replace(`${source}.`, '') : id;

  return {
    labelKey,
    source,
  };
};

const getColumnHeader = (labelKey: string, source?: string) => {
  if (source === BROWSER_SWARMING_SOURCE) {
    return `sw.${labelKey}`;
  } else if (source === BROWSER_UFS_SOURCE) {
    return `ufs.${labelKey}`;
  } else {
    return labelKey;
  }
};

export const getBrowserColumnIds = (
  dimensions?: GetBrowserDeviceDimensionsResponse,
  extraColumns: string[] = [],
): string[] => {
  const ids: string[] = [];
  if (dimensions) {
    ids.push(
      ...Object.keys(dimensions.baseDimensions)
        .concat(
          ...Object.keys(dimensions.swarmingLabels).map(
            (l) => `${BROWSER_SWARMING_SOURCE}.${l}`,
          ),
        )
        .concat(
          ...Object.keys(dimensions.ufsLabels).map(
            (l) => `${BROWSER_UFS_SOURCE}.${l}`,
          ),
        ),
    );
  }

  ids.push(...Object.keys(CUSTOM_COLUMNS));
  ids.push('realm');
  ids.push(...extraColumns);
  return _.uniq(ids);
};

export const getBrowserColumn = (id: string): BrowserColumnDef => {
  const customColumn = CUSTOM_COLUMNS[id];
  if (customColumn) {
    return customColumn;
  }

  const { labelKey, source } = destructureColumnId(id);

  return {
    accessorKey: id,
    header: getColumnHeader(labelKey, source),
    orderByField: id,
    filterByField: source ? `${source}."${labelKey}"` : id,
    enableEditing: false,
    minSize: 70,
    maxSize: 700,
    enableSorting: true,
    accessorFn: (device) => {
      let values: readonly string[] | undefined = undefined;

      if (source === BROWSER_SWARMING_SOURCE) {
        values = device.swarmingLabels?.[labelKey]?.values;
      } else if (source === BROWSER_UFS_SOURCE) {
        values = device.ufsLabels?.[labelKey]?.values;
      }

      return values ? labelValuesToString(values) : undefined;
    },
    Cell: (param) => (
      <EllipsisTooltip>
        {(param.renderedCellValue as React.ReactNode) ?? ''}
      </EllipsisTooltip>
    ),
    ...(BROWSER_COLUMN_OVERRIDES[id] ?? {}),
  };
};
