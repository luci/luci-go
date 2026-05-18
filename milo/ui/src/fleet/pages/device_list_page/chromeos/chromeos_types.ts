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

import { MRT_ColumnDef } from 'material-react-table';
import React from 'react';

import { FC_CellProps } from '@/fleet/types/table';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { TaskResult } from './use_chromeos_current_tasks';

/**
 * Categorizes the fields for a ChromeOS device.
 * - 'base': Direct properties of the Device object (e.g., id, state).
 * - 'label': Custom labels stored in deviceSpec.labels (e.g., model, board).
 * - 'special': Computed or externally fetched fields (e.g., current_task).
 */
export type FieldType = 'base' | 'label' | 'special';

export interface BaseFieldDefinition {
  type: 'base';
  header?: string;
  accessorFn: (device: ChromeOSDevice) => unknown;
  orderByField?: string;
  filterKey?: string;
  renderCell?: (props: FC_CellProps<ChromeOSDevice>) => React.ReactNode;
}

export interface LabelFieldDefinition {
  type: 'label';
  header?: string;
  accessorFn?: (device: ChromeOSDevice) => unknown;
  orderByField?: string;
  filterKey?: string;
  renderCell?: (props: FC_CellProps<ChromeOSDevice>) => React.ReactNode;
}

export interface SpecialFieldDefinition {
  type: 'special';
  header: string;
  accessorFn: (device: ChromeOSDevice) => unknown;
  orderByField?: string;
  filterKey?: string;
  renderCell?: (props: FC_CellProps<ChromeOSDevice>) => React.ReactNode;
}

export type FieldDefinition =
  | BaseFieldDefinition
  | LabelFieldDefinition
  | SpecialFieldDefinition;

export type ChromeOSDevice = Device & { current_task?: TaskResult };

export type ChromeOSColumnDef = Omit<MRT_ColumnDef<ChromeOSDevice>, 'id'> & {
  id: string;
  orderByField?: string;
  filterKey?: string;
};
