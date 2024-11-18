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

export const BASE_DIMENSIONS = [
  'id', // added for compliance, later will get from the backend
  // 'task', // should come from backend
  'externalIp',
  'lastSeen',
  'firstSeen',
  'version',
  // Derived
  'disk_space',
  'uptime',
  'running_time',
  // 'status', // should come from backend
  'internal_ip',
  'battery_level',
  'battery_voltage',
  'battery_temperature',
  'battery_status',
  'battery_health',
  'bot_temperature',
  'device_temperature',
  'serial_number',
];

export const COLUMN_HEADERS: { [key: string]: string } = {
  id: 'Bot Id',
  externalIp: 'External IP',
  firstSeen: 'First Seen',
  lastSeen: 'Last Seen',
  version: 'Client Code Version',
  battery_health: 'Battery Health',
  battery_level: 'Battery Level (%)',
  battery_status: 'Battery Status',
  battery_temperature: 'Battery Temp (°C)',
  battery_voltage: 'Battery Voltage (mV)',
  bot_temperature: 'Bot Temp (°C)',
  device_temperature: 'Device Temp (°C)',
  disk_space: 'Free Space (MB)',
  internal_ip: 'Internal or Local IP',
  running_time: 'Swarming Uptime',
  serial_number: 'Device Serial Number',
  uptime: 'Bot Uptime',
};
