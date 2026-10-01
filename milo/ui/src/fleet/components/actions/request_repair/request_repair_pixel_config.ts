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

import {
  generateDeviceDetailsURL,
  PIXEL_PLATFORM,
} from '@/fleet/constants/paths';
import { md, rawMd } from '@/fleet/utils/markdown_utils';
import { getMobileHarnessLink } from '@/fleet/utils/mobile_harness';
import { AndroidDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { RepairConfig } from './request_repair';

export const PIXEL_RESTORATION_COMPONENT_ID = '2174751';
export const PIXEL_RESTORATION_TEMPLATE_ID = '2403455';

export const extractPixelLabName = (device: AndroidDevice): string => {
  const lab =
    device.omnilabSpec?.labels?.['lab_name']?.values?.[0] ||
    device.omnilabSpec?.labels?.['lab_location']?.values?.[0] ||
    '';
  return lab.trim();
};

export const areDevicesFromSingleLab = (devices: AndroidDevice[]): boolean => {
  if (devices.length === 0) return false;
  const labs = devices.map(extractPixelLabName);
  if (labs.some((lab) => !lab)) {
    return false;
  }
  const uniqueLabs = new Set(labs);
  return uniqueLabs.size === 1;
};

export { getMobileHarnessLink };

export const generatePixelRestorationTitle = (
  devices: AndroidDevice[],
): string => {
  if (!devices.length) throw new Error('No devices specified');
  if (!areDevicesFromSingleLab(devices)) {
    throw new Error('Selected devices must belong to the same lab');
  }
  const lab = extractPixelLabName(devices[0]);
  if (devices.length === 1) {
    return `[${lab}][Restoration][${devices[0].id}]`;
  }
  if (devices.length === 2) {
    return `[${lab}][Restoration][${devices[0].id}, ${devices[1].id}]`;
  }
  return `[${lab}][Restoration] [${devices.length}] - [Multiple Devices]`;
};

export const generatePixelDeviceDetails = (device: AndroidDevice): string => {
  const fcUrl = rawMd(generateDeviceDetailsURL(PIXEL_PLATFORM, device.id));
  const mhUrl = rawMd(getMobileHarnessLink(device));
  const hostname =
    device.omnilabSpec?.labels?.['hostname']?.values?.[0] ||
    device.omnilabSpec?.labels?.['host_name']?.values?.[0];
  const hostIp = device.omnilabSpec?.labels?.['host_ip']?.values?.[0];
  const state =
    device.omnilabSpec?.labels?.['dut_state']?.values?.[0] ||
    device.omnilabSpec?.labels?.['state']?.values?.[0];
  const isOffline = device.omnilabSpec?.labels?.['fc_is_offline']?.values?.[0];
  const locationTag =
    device.omnilabSpec?.labels?.['location_tag']?.values?.[0] ||
    device.omnilabSpec?.labels?.['ufs.location_tag']?.values?.[0];
  const runTarget =
    device.runTarget ||
    device.omnilabSpec?.labels?.['product_board']?.values?.[0] ||
    device.omnilabSpec?.labels?.['hardware']?.values?.[0];

  const lines = [
    md`* **Device:** [${device.id}](${fcUrl})`,
    hostname ? md`  * **Hostname:** ${hostname}` : '',
    hostIp ? md`  * **Host IP:** ${hostIp}` : '',
    runTarget ? md`  * **Run Target:** ${runTarget}` : '',
    state ? md`  * **State:** ${state}` : '',
    isOffline !== undefined ? md`  * **Offline:** ${isOffline}` : '',
    device.fcOfflineSince
      ? md`  * **Offline Since:** ${device.fcOfflineSince}`
      : '',
    locationTag ? md`  * **Location Tag:** ${locationTag}` : '',
    md`  * **Mobile Harness:** [View in Mobile Harness](${mhUrl})`,
  ].filter(Boolean);

  return lines.join('\n');
};

export const generatePixelDevicesTable = (
  devices: AndroidDevice[],
  includeLinks = true,
): string => {
  const headers = [
    'Device ID',
    'Hostname',
    'Run Target',
    'State',
    'Mobile Harness',
  ];
  const headerRow = `| ${headers.join(' | ')} |`;
  const alignRow = `| ${headers.map(() => '---').join(' | ')} |`;

  const rows = devices.map((d) => {
    const hostname =
      d.omnilabSpec?.labels?.['hostname']?.values?.[0] ||
      d.omnilabSpec?.labels?.['host_name']?.values?.[0] ||
      '';
    const state =
      d.omnilabSpec?.labels?.['dut_state']?.values?.[0] ||
      d.omnilabSpec?.labels?.['state']?.values?.[0] ||
      '';
    const runTarget =
      d.runTarget ||
      d.omnilabSpec?.labels?.['product_board']?.values?.[0] ||
      d.omnilabSpec?.labels?.['hardware']?.values?.[0] ||
      '';

    const mhUrl = rawMd(getMobileHarnessLink(d));

    if (!includeLinks) {
      return md`| ${d.id} | ${hostname} | ${runTarget} | ${state} | [View in MH](${mhUrl}) |`;
    }

    const fcUrl = rawMd(generateDeviceDetailsURL(PIXEL_PLATFORM, d.id));
    return md`| [${d.id}](${fcUrl}) | ${hostname} | ${runTarget} | ${state} | [View in MH](${mhUrl}) |`;
  });

  return [headerRow, alignRow, ...rows].join('\n');
};

export const generatePixelRestorationDescription = (
  devices: AndroidDevice[],
): string => {
  if (!devices.length) throw new Error('No devices specified');
  if (!areDevicesFromSingleLab(devices)) {
    throw new Error('Selected devices must belong to the same lab');
  }

  const lab = extractPixelLabName(devices[0]);

  const filterString = devices.map((d) => `id = "${d.id}"`).join(' OR ');
  const encodedFilters = encodeURIComponent(filterString);
  const isFiltersTooLong = encodedFilters.length > 1000;
  const fconUrl = rawMd(
    isFiltersTooLong
      ? 'https://ci.chromium.org/ui/fleet/p/pixel/devices'
      : `https://ci.chromium.org/ui/fleet/p/pixel/devices?filters=${encodedFilters}`,
  );

  const buildDescription = (mode: 'full' | 'compact' | 'minimal'): string => {
    const parts: string[] = [
      md`Lab: ${lab}`,
      '',
      md`### Selected Devices for Restoration (${devices.length} ${devices.length === 1 ? 'device' : 'devices'})`,
      '',
    ];

    if (devices.length === 1) {
      parts.push(generatePixelDeviceDetails(devices[0]));
    } else {
      const fconLinkText = isFiltersTooLong
        ? md`[View selected devices in Fleet Console](${fconUrl}) (Device filter omitted due to length)`
        : md`[View selected devices in Fleet Console](${fconUrl})`;
      parts.push(fconLinkText);
      parts.push('');

      if (mode === 'full') {
        parts.push(generatePixelDevicesTable(devices, true));
        if (devices.length <= 5) {
          parts.push('');
          parts.push('### Device Details');
          parts.push('');
          parts.push(devices.map(generatePixelDeviceDetails).join('\n'));
        }
      } else if (mode === 'compact') {
        parts.push(generatePixelDevicesTable(devices, false));
      } else {
        parts.push(md`**Device IDs:** ${devices.map((d) => d.id).join(', ')}`);
      }
    }

    return parts.join('\n');
  };

  let description = buildDescription('full');
  if (description.length > 3500) {
    description = buildDescription('compact');
  }
  if (description.length > 3500) {
    description = buildDescription('minimal');
  }

  return description;
};

export const PixelRestorationConfig: RepairConfig<AndroidDevice> = {
  componentId: PIXEL_RESTORATION_COMPONENT_ID,
  getTemplateId: () => PIXEL_RESTORATION_TEMPLATE_ID,
  generateTitle: generatePixelRestorationTitle,
  generateDescription: generatePixelRestorationDescription,
  baseUrl: 'https://b.corp.google.com',
};
