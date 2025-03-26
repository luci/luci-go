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

import { stringifyFilters } from '../components/filter_dropdown/search_param_utils/search_param_utils';

// this is loosely defining default columns based on b/391621656
// Swarming labels are dynamic, but for specific views, such as FLOPS'
// ChromeOS device view, commonly used labels are known. The code will
// ignore these columns if they don't match any existing entries.
export const DEFAULT_DEVICE_COLUMNS: string[] = [
  'id',
  'dut_id',
  'state',
  'dut_state',
  'label-board',
  'label-model',
  'label-phase',
  'label-pool',
  'label-servo_component',
  'label-servo_state',
  'label-servo_usb_state',
];

// Define a list of device filters commonly used by FLOPS to show in the
// filter options for the device.
export const COMMON_DEVICE_FILTERS: string[] = [
  'dut_state',
  'label-board',
  'label-model',
  'label-pool',
  'label-phase',
];

/**
 * Generate a link to find a particular device by its dut_name Swarming label.
 * Technically, ID is the same as dut_name for ChromeOS - but we might not
 * always be able to assume this in the future.
 */
// TODO: b/402410880 - Consider making it possible to directly use the id of a
// device for a URL in the future.
export const generateDutNameRedirectURL = (dutName: string): string => {
  return `/ui/fleet/redirects/singledevice?filters=${stringifyFilters({ 'labels.dut_name': [dutName] })}`;
};
