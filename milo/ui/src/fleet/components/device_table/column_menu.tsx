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

import {
  GridColumnMenu,
  GridColumnMenuHideItem,
  GridColumnMenuItemProps,
  GridColumnMenuProps,
} from '@mui/x-data-grid';

import { getFeatureFlag } from '@/fleet/config/features';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { FilterItem } from './filter_item';

export type ColumnMenuProps = GridColumnMenuProps & {
  platform: Platform;
};

function ColumnsItem(props: GridColumnMenuItemProps) {
  return <GridColumnMenuHideItem {...props} />;
}

/**
 * Customized implementation of GridColumnMenu for fleet use cases.
 */
export function ColumnMenu({ platform, ...props }: ColumnMenuProps) {
  const { colDef } = props;
  const hideable = colDef.hideable ?? true;

  return (
    <GridColumnMenu
      {...props}
      slots={{
        // Use custom filtering widget.
        columnMenuFilterItem: getFeatureFlag('ColumnFilter')
          ? FilterItem
          : null,
        // TODO: b/393602662 - Column menu items are disabled because they're
        // different from our custom column component. We can re-add this later
        // if we can make sure state is synced and the components are aligned.
        columnMenuColumnsItem: hideable ? ColumnsItem : null,
      }}
      slotProps={{ columnMenuFilterItem: { platform: platform } }}
    />
  );
}
