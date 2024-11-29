// Copyright 2024 The LUCI Authors.
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
  Device,
  DeviceState,
  DeviceType,
} from '@/proto/infra/fleetconsole/api/fleetconsolerpc/service.pb';

interface Dimension {
  id: string, // unique id used for sorting and filtering
  displayName: string,
  getValue: (device: Device) => string
}

export const BASE_DIMENSIONS : Dimension[] = [
  {
    id: 'id',
    displayName: 'ID',
    getValue: (device: Device) => device.id,
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
    displayName: 'State',
    getValue: (device: Device) => DeviceState[device.state],
  },
  {
    id: 'address.host',
    displayName: 'Address',
    getValue: (device: Device) => device.address?.host || '',
  },
  {
    id: 'address.port',
    displayName: 'Port',
    getValue: (device: Device) => String(device.address?.port) || '',
  }
]