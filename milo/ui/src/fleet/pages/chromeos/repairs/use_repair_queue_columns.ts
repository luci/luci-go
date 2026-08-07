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

import { useMemo } from 'react';

import {
  ChromeOSColumnDef,
  getFieldDefinition,
} from '@/fleet/pages/device_list_page/chromeos/chromeos_fields';

export const REPAIR_QUEUE_COLUMN_IDS = [
  'dut_id',
  'label-pool',
  'label-model',
  'dut_state',
] as const;

export const useRepairQueueColumns = () => {
  const columns: ChromeOSColumnDef[] = useMemo(() => {
    return [
      getFieldDefinition('dut_id').columnDef,
      getFieldDefinition('label-pool').columnDef,
      getFieldDefinition('label-model').columnDef,
      {
        ...getFieldDefinition('dut_state').columnDef,
        header: 'State',
      },
    ];
  }, []);

  return { columns };
};
