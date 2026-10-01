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

export const ANDROID_DEFAULT_COLUMNS = [
  'id',
  'state',
  'device_type',
  'build',
  'host_group',
  'hostname',
  'model',
  'pool',
  'run_target',
  'type',
  'version',
];

export const BROWSER_DEFAULT_COLUMNS = [
  'id',
  'ufs.hostname',
  'ufs.resource_state',
  'ufs.zone',
  'ufs.chrome_platform',
  'ufs.serial_number',
  'ufs.associated_hostname',
  'ufs.model',
  'sw.device_os',
  'sw.device_type',
  'sw.device_state',
  'sw.dut_state',
  'sw.os',
  'sw.pool',
  'sw.state',
  'realm',
];
