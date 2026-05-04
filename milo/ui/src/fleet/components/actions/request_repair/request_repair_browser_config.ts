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

import { RepairConfig } from './request_repair';
import {
  extractHostname,
  FLEET_CONSOLE_TRACKING_HOTLIST,
} from './request_repair_utils';

export interface BrowserDeviceToRepair {
  id: string; // Machine name
  hostname?: string;
  pool?: string;
  zone?: string;
  serialNumber?: string;
  type?: string;
  model?: string;
  status?: string;
  lastSeen?: string;
}

const ZONE_HOTLIST_MAP: Record<string, string> = {
  SFO36_BROWSER: '6458008',
  IAD65_BROWSER: '6909561',
  ATL97_BROWSER: '6909561',
};

const generateBrowserTitle = (devices: BrowserDeviceToRepair[]): string => {
  if (!devices.length) return '';

  const zones = Array.from(
    new Set(devices.map((d) => d.zone?.toUpperCase()).filter(Boolean)),
  );
  const zone =
    zones.length === 1
      ? zones[0]
      : zones.length > 1
        ? `${zones.length} zones`
        : 'Unknown Zone';
  const hostnames = devices.map((d) => extractHostname(d));

  if (devices.length === 1) {
    return `[${zone}][Browser][Repair][${hostnames[0]}]`;
  }
  if (devices.length === 2) {
    return `[${zone}][Browser][Repair][${hostnames[0]}, ${hostnames[1]}]`;
  }
  return `[${zone}][Browser] [Repair] [${devices.length}] - [Multiple Devices]`;
};

const generateBrowserIssueDescription = (dev: BrowserDeviceToRepair) => {
  const hostname = extractHostname(dev);
  const machineName = dev.id;
  const url = `https://ci.chromium.org/ui/fleet/p/chromium/devices/${machineName}`;

  const poolUrl = dev.pool
    ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."pool" = "${dev.pool}"`)}`
    : '';
  const zoneUrl = dev.zone
    ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`ufs."zone" = "${dev.zone}"`)}`
    : '';
  const modelUrl = dev.model
    ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."model" = "${dev.model}"`)}`
    : '';
  const statusUrl = dev.status
    ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."status" = "${dev.status}"`)}`
    : '';
  const typeUrl = dev.type
    ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."type" = "${dev.type}"`)}`
    : '';
  const serialUrl = dev.serialNumber
    ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`ufs."serialNumber" = "${dev.serialNumber}"`)}`
    : '';

  const details = [
    `    * **Machine:** [${machineName}](${url})`,
    dev.serialNumber
      ? `    * **Serial Number:** [${dev.serialNumber}](${serialUrl})`
      : '',
    dev.type ? `    * **Type:** [${dev.type}](${typeUrl})` : '',
    dev.model ? `    * **Model:** [${dev.model}](${modelUrl})` : '',
    dev.status ? `    * **Status:** [${dev.status}](${statusUrl})` : '',
    dev.pool ? `    * **Pool:** [${dev.pool}](${poolUrl})` : '',
    dev.zone ? `    * **Zone:** [${dev.zone}](${zoneUrl})` : '',
    dev.lastSeen ? `    * **Last Seen:** ${dev.lastSeen}` : '',
  ]
    .filter(Boolean)
    .join('\n');

  const description = [
    '**Instructions on how to create a bug: go/flops-browser-escalations/. ' +
      'Your request will automatically be placed in work queue for repair. ' +
      'Please do not explicitly assign bugs to individuals without prior discussion with said individuals. ' +
      'Reminder to update both title and description.**',
    '',
    '---',
    '',
    `* **Hostname:** ${hostname}`,
    details,
    '',
    '**Issue / Request:**',
    '',
    '**Logs (if applicable):**',
    '',
    '---------------------',
    '',
    '**Additional Info (Optional):**',
  ].join('\n');
  return description;
};

const generateBrowserBulkIssueDescription = (
  devices: BrowserDeviceToRepair[],
) => {
  const linkedDevices = devices
    .map((dev) => {
      const hostname = extractHostname(dev);
      const url = `https://ci.chromium.org/ui/fleet/p/chromium/devices/${dev.id}`;
      const poolUrl = dev.pool
        ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."pool" = "${dev.pool}"`)}`
        : '';
      const zoneUrl = dev.zone
        ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`ufs."zone" = "${dev.zone}"`)}`
        : '';
      const modelUrl = dev.model
        ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."model" = "${dev.model}"`)}`
        : '';
      const statusUrl = dev.status
        ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."status" = "${dev.status}"`)}`
        : '';
      const typeUrl = dev.type
        ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."type" = "${dev.type}"`)}`
        : '';
      const serialUrl = dev.serialNumber
        ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`ufs."serialNumber" = "${dev.serialNumber}"`)}`
        : '';

      const details = [
        `    * **Machine:** [${dev.id}](${url})`,
        dev.serialNumber
          ? `    * **Serial Number:** [${dev.serialNumber}](${serialUrl})`
          : '',
        dev.type ? `    * **Type:** [${dev.type}](${typeUrl})` : '',
        dev.model ? `    * **Model:** [${dev.model}](${modelUrl})` : '',
        dev.status ? `    * **Status:** [${dev.status}](${statusUrl})` : '',
        dev.pool ? `    * **Pool:** [${dev.pool}](${poolUrl})` : '',
        dev.zone ? `    * **Zone:** [${dev.zone}](${zoneUrl})` : '',
        dev.lastSeen ? `    * **Last Seen:** ${dev.lastSeen}` : '',
      ]
        .filter(Boolean)
        .join('\n');

      return `* **Hostname:** ${hostname}\n${details}`;
    })
    .join('\n');

  const description = [
    '**Requester: Instructions on how to create a bug: go/flops-browser-escalations/. ' +
      'Your request will automatically be placed in work queue for repair. ' +
      'Please do not explicitly assign bugs to individuals without prior discussion with said individuals. ' +
      'Reminder to update both title and description.**',
    '',
    '---',
    '',
    '**Devices:**',
    `${linkedDevices}`,
    '',
    '**Issue / Request:**',
    '',
    '**Logs & Swarming link:**',
    '',
    '---------------------',
    '',
    '**Additional Info (Optional):**',
  ].join('\n');
  return description;
};

export const BrowserRepairConfig: RepairConfig<BrowserDeviceToRepair> = {
  componentId: '1735976',
  getTemplateId: (items) => (items.length === 1 ? '2107381' : '2161122'),
  generateTitle: generateBrowserTitle,
  generateDescription: (items) => {
    if (items.length === 1) {
      return generateBrowserIssueDescription(items[0]);
    }
    return generateBrowserBulkIssueDescription(items);
  },
  hotlistIds: (items) => {
    const hotlists = new Set<string>();

    items.forEach((item) => {
      if (item.zone) {
        const zoneUpper = item.zone.toUpperCase();
        if (ZONE_HOTLIST_MAP[zoneUpper]) {
          hotlists.add(ZONE_HOTLIST_MAP[zoneUpper]);
        }
      }
    });

    hotlists.add(FLEET_CONSOLE_TRACKING_HOTLIST);

    return Array.from(hotlists).join(',');
  },
};
