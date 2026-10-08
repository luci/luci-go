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

const ACRONYMS: Record<string, string> = {
  id: 'ID',
  dut: 'DUT',
  os: 'OS',
  ip: 'IP',
  mac: 'MAC',
  ec: 'EC',
  ap: 'AP',
  ro: 'RO',
  rw: 'RW',
  sku: 'SKU',
  hwid: 'HWID',
  gsc: 'GSC',
  rpm: 'RPM',
  ufs: 'UFS',
  usb: 'USB',
  gpu: 'GPU',
  cpu: 'CPU',
  ram: 'RAM',
  ssd: 'SSD',
  wifi: 'Wi-Fi',
  bt: 'BT',
  imei: 'IMEI',
  sim: 'SIM',
  uuid: 'UUID',
  url: 'URL',
  vm: 'VM',
  abi: 'ABI',
  sdk: 'SDK',
  adb: 'ADB',
  sw: 'SW',
  dms: 'DMS',
  mh: 'MH',
};

/**
 * Formats a raw dimension or column key (e.g. `label-servo_state`, `dut_state`,
 * `sw_version`, `dms.pool`) into a human-readable fallback header label
 * (`Servo State`, `DUT State`, `SW Version`, `DMS Pool`) while preserving
 * acronyms.
 *
 * Platform-specific display names should be configured in the platform's field
 * definitions rather than hardcoded here.
 */
export function humanizeColumnLabel(raw: string): string {
  if (!raw) return raw;

  const stripped = raw
    .replace(/^label-/, '')
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2');

  return stripped
    .split(/[\s._-]+/)
    .filter(Boolean)
    .map((word) => {
      const lower = word.toLowerCase();
      if (ACRONYMS[lower]) {
        return ACRONYMS[lower];
      }
      return lower.charAt(0).toUpperCase() + lower.slice(1);
    })
    .join(' ');
}
