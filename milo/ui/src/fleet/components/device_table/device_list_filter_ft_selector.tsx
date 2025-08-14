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

import { OptionCategory, SelectedOptions } from '@/fleet/types';

import { DeviceListFilterBar as DeviceListFilterBarNew } from './device_list_filter_bar';
import { DeviceListFilterBarOld } from './device_list_filter_bar_old';

const ENABLE_UNIFIED_FILTER_BAR = SETTINGS.fleetConsole.enableUnifiedFilterBar;

export const DeviceListFilterBar = (props: {
  filterOptions: OptionCategory[];
  selectedOptions: SelectedOptions;
  onSelectedOptionsChange: (newSelectedOptions: SelectedOptions) => void;
  isLoading?: boolean;
}) => {
  if (ENABLE_UNIFIED_FILTER_BAR) {
    return <DeviceListFilterBarNew {...props} />;
  } else {
    return <DeviceListFilterBarOld {...props} />;
  }
};
