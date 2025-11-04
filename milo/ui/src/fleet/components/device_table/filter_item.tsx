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

import FilterListIcon from '@mui/icons-material/FilterList';
import {
  ListItemIcon,
  MenuItem,
  PopoverOrigin,
  Typography,
} from '@mui/material';
import { GridColumnMenuItemProps, useGridApiContext } from '@mui/x-data-grid';
import { useEffect, useState } from 'react';

import { OptionsDropdown } from '@/fleet/components/options_dropdown';
import { OptionValue } from '@/fleet/types/option';
import { fuzzySort } from '@/fleet/utils/fuzzy_sort';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  GetDeviceDimensionsResponse,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { useDeviceDimensions } from '../../pages/device_list_page/common/use_device_dimensions';
import { MenuSkeleton } from '../filter_dropdown/menu_skeleton';
import { OptionsMenuOld } from '../filter_dropdown/options_menu_old';
import {
  filtersUpdater,
  getFilters,
} from '../filter_dropdown/search_param_utils';

export type FilterItemProps = GridColumnMenuItemProps & {
  platform: Platform;
};

const getDimensionInfo = (
  dimensionsData: GetDeviceDimensionsResponse,
  field: string,
): { dimensionValues: readonly string[]; filterKey: string } | null => {
  if (dimensionsData.baseDimensions[field]) {
    return {
      dimensionValues: dimensionsData.baseDimensions[field].values,
      filterKey: field,
    };
  }
  if (dimensionsData.labels[field]) {
    return {
      dimensionValues: dimensionsData.labels[field].values,
      filterKey: `labels."${field}"`, // TODO: Hotfix for b/449956551, needs further investigation on quote handling
    };
  }
  return null;
};

export function FilterItem({ platform, ...props }: FilterItemProps) {
  const apiRef = useGridApiContext();
  const { colDef } = props;
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const [anchorOrigin, setAnchorOrigin] = useState<PopoverOrigin>({
    vertical: 'top',
    horizontal: 'right',
  });
  const [transformOrigin, setTransformOrigin] = useState<PopoverOrigin>({
    vertical: 'top',
    horizontal: 'left',
  });
  const { data: dimensionsData, isLoading } = useDeviceDimensions({
    platform,
  });
  const [searchParams, setSearchParams] = useSyncedSearchParams();

  const dimensionInfo = dimensionsData
    ? getDimensionInfo(dimensionsData, colDef.field)
    : null;
  const isFilterable = !!dimensionInfo;
  const filterKey = dimensionInfo?.filterKey;

  const [selectedValues, setSelectedValues] = useState<Set<string>>(new Set());

  const open = Boolean(anchorEl);

  useEffect(() => {
    if (open && filterKey) {
      const existingFilters = getFilters(searchParams).filters || {};
      setSelectedValues(new Set(existingFilters[filterKey] || []));
    }
  }, [open, filterKey, searchParams]);

  const handleOpen = (event: React.MouseEvent<HTMLElement>) => {
    const rect = event.currentTarget.getBoundingClientRect();
    const screenWidth = window.innerWidth;
    // A reasonable guess for the dropdown width.
    const dropdownWidth = 300;

    // Decide which way to open the dropdown based on available space.
    if (rect.right + dropdownWidth > screenWidth && rect.left > dropdownWidth) {
      // Not enough space on the right, so open to the left.
      setAnchorOrigin({ vertical: 'top', horizontal: 'left' });
      setTransformOrigin({ vertical: 'top', horizontal: 'right' });
    } else {
      // Default behavior: open to the right.
      setAnchorOrigin({ vertical: 'top', horizontal: 'right' });
      setTransformOrigin({ vertical: 'top', horizontal: 'left' });
    }
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
    // Close the parent column menu.
    apiRef.current.hideColumnMenu();
  };

  const handleApply = () => {
    if (!filterKey) return;
    const existingFilters = getFilters(searchParams).filters || {};
    const newFilters = {
      ...existingFilters,
      [filterKey]: Array.from(selectedValues),
    };

    if (selectedValues.size === 0) {
      delete newFilters[filterKey];
    }

    setSearchParams(filtersUpdater(newFilters));
    handleClose();
  };

  const handleReset = () => {
    setSelectedValues(new Set());
  };

  const flipOption = (value: string) => {
    const newSelectedValues = new Set(selectedValues);
    if (newSelectedValues.has(value)) {
      newSelectedValues.delete(value);
    } else {
      newSelectedValues.add(value);
    }
    setSelectedValues(newSelectedValues);
  };

  const options: OptionValue[] =
    dimensionInfo?.dimensionValues.map((v) => ({ value: v, label: v })) || [];

  return (
    <>
      <MenuItem onClick={handleOpen} disabled={!isFilterable}>
        <ListItemIcon>
          <FilterListIcon fontSize="small" />
        </ListItemIcon>
        <Typography variant="inherit">Filter</Typography>
      </MenuItem>
      <OptionsDropdown
        open={open}
        anchorEl={anchorEl}
        onClose={handleClose}
        anchorOrigin={anchorOrigin}
        transformOrigin={transformOrigin}
        enableSearchInput={true}
        maxHeight={500}
        onResetClick={handleReset}
        footerButtons={['reset', 'cancel', 'apply']}
        onApply={handleApply}
        renderChild={(searchQuery) => {
          if (isLoading) {
            return <MenuSkeleton itemCount={10} maxHeight={400} />;
          }
          if (!dimensionInfo) {
            return null;
          }
          const sortedOptions = fuzzySort(searchQuery)(options, (x) => x.label);
          return (
            <OptionsMenuOld
              elements={sortedOptions}
              selectedElements={selectedValues}
              flipOption={flipOption}
            />
          );
        }}
      />
    </>
  );
}
