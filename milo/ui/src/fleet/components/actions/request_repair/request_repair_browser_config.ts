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

const generateDeviceDetails = (dev: BrowserDeviceToRepair): string => {
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
  const details = [
    `    * **Machine:** [${dev.id}](${url})`,
    dev.serialNumber ? `    * **Serial Number:** ${dev.serialNumber}` : '',
    dev.type ? `    * **Type:** [${dev.type}](${typeUrl})` : '',
    dev.model ? `    * **Model:** [${dev.model}](${modelUrl})` : '',
    dev.status ? `    * **Status:** [${dev.status}](${statusUrl})` : '',
    dev.pool ? `    * **Pool:** [${dev.pool}](${poolUrl})` : '',
    dev.zone ? `    * **Zone:** [${dev.zone}](${zoneUrl})` : '',
    dev.lastSeen ? `    * **Last Seen:** ${dev.lastSeen}` : '',
  ]
    .filter(Boolean)
    .join('\n');

  return details;
};

const generateBrowserTable = (
  devices: BrowserDeviceToRepair[],
  includeLinks = false,
): string => {
  const headers = ['Hostname', 'Machine', 'Serial', 'Pool', 'Zone', 'Status'];
  const headerRow = `| ${headers.join(' | ')} |`;
  const alignRow = `| ${headers.map(() => '---').join(' | ')} |`;

  const rows = devices.map((dev) => {
    const hostname = extractHostname(dev);

    if (!includeLinks) {
      return `| ${hostname} | ${dev.id} | ${dev.serialNumber || ''} | ${dev.pool || ''} | ${dev.zone || ''} | ${dev.status || ''} |`;
    }

    const url = `https://ci.chromium.org/ui/fleet/p/chromium/devices/${dev.id}`;
    const poolUrl = dev.pool
      ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`sw."pool" = "${dev.pool}"`)}`
      : '';
    const zoneUrl = dev.zone
      ? `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodeURIComponent(`ufs."zone" = "${dev.zone}"`)}`
      : '';

    const machineCell = `[${dev.id}](${url})`;
    const serialCell = dev.serialNumber || '';
    const poolCell = dev.pool ? `[${dev.pool}](${poolUrl})` : '';
    const zoneCell = dev.zone ? `[${dev.zone}](${zoneUrl})` : '';

    return `| ${hostname} | ${machineCell} | ${serialCell} | ${poolCell} | ${zoneCell} | ${dev.status || ''} |`;
  });

  return [headerRow, alignRow, ...rows].join('\n');
};

/**
 * Generates the Buganizer issue description for a single device.
 * @param dev The device to repair/reinstall.
 * @param actionType The type of action ('repair' | 'reinstall').
 */
const generateBrowserIssueDescription = (
  dev: BrowserDeviceToRepair,
  actionType: 'repair' | 'reinstall',
) => {
  const hostname = extractHostname(dev);
  const details = generateDeviceDetails(dev);

  let requestText = '';
  if (actionType === 'reinstall') {
    // TODO(b/477806570): Add dynamic OS selection or detection.
    requestText =
      'Please upgrade the following device to OS <TARGET_OS> (Fill in target OS):';
  } else {
    requestText = 'Please repair the following device:';
  }

  const escalationsLink =
    actionType === 'reinstall'
      ? 'go/browser-repair-escalate'
      : 'go/flops-browser-escalations/';

  const description = [
    `**Instructions on how to create a bug: ${escalationsLink}. ` +
      'Your request will automatically be placed in work queue for repair. ' +
      'Please do not explicitly assign bugs to individuals without prior discussion with said individuals. ' +
      'Reminder to update both title and description.**',
    '',
    '---',
    '',
    requestText,
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

const generateZonalHostList = (devices: BrowserDeviceToRepair[]): string => {
  const zoneMap: Record<string, string[]> = {};
  devices.forEach((d) => {
    const zone = d.zone?.toUpperCase() || 'UNKNOWN';
    const hostname = extractHostname(d);
    if (!zoneMap[zone]) {
      zoneMap[zone] = [];
    }
    zoneMap[zone].push(hostname);
  });

  return Object.entries(zoneMap)
    .map(([zone, hosts]) => `* **${zone}**: ${hosts.join(' ')}`)
    .join('\n');
};

/**
 * Generates the Buganizer issue description for multiple devices.
 * Includes a zonal summary, detailed list, and a link to view devices in FCon.
 * @param devices The list of devices.
 * @param actionType The type of action ('repair' | 'reinstall').
 */
const generateBrowserBulkIssueDescription = (
  devices: BrowserDeviceToRepair[],
  actionType: 'repair' | 'reinstall',
) => {
  const zonalList = generateZonalHostList(devices);

  const filterString = devices.map((d) => `id = "${d.id}"`).join(' OR ');
  const encodedFilters = encodeURIComponent(filterString);
  const isTooLong = encodedFilters.length > 1000;

  const fallbackUrl =
    typeof window !== 'undefined'
      ? `${window.location.origin}${window.location.pathname}${window.location.search}`
      : 'https://ci.chromium.org/ui/fleet/p/chromium/devices';

  const fconUrl = isTooLong
    ? fallbackUrl
    : `https://ci.chromium.org/ui/fleet/p/chromium/devices?filters=${encodedFilters}`;

  const createDescription = (mode: 'full' | 'compact' | 'minimal') => {
    const table =
      mode === 'minimal' ? '' : generateBrowserTable(devices, mode === 'full');

    let requestText = '';
    if (actionType === 'reinstall') {
      // TODO(b/477806570): Add dynamic OS selection or detection.
      requestText =
        'Please upgrade the following devices to OS <TARGET_OS> (Fill in target OS):';
    } else {
      requestText = 'Please repair the following devices:';
    }

    const escalationsLink =
      actionType === 'reinstall'
        ? 'go/browser-repair-escalate'
        : 'go/flops-browser-escalations/';

    const descriptionArray = [
      `**Requester: Instructions on how to create a bug: ${escalationsLink}. ` +
        'Your request will automatically be placed in work queue for repair. ' +
        'Please do not explicitly assign bugs to individuals without prior discussion with said individuals. ' +
        'Reminder to update both title and description.**',
      '',
      '---',
      '',
      requestText,
      '',
      '**Devices (Zonal Summary):**',
      zonalList,
      '',
    ];

    if (mode !== 'minimal') {
      descriptionArray.push('**Devices:**', '', table, '');
    }

    descriptionArray.push(
      isTooLong
        ? `[View all](${fconUrl}) (Specific devices omitted due to URL length limit)`
        : `[View all](${fconUrl})`,
      '',
      '**Issue / Request:**',
      '',
      '**Logs & Swarming link:**',
      '',
      '---------------------',
      '',
      '**Additional Info (Optional):**',
    );

    return descriptionArray.join('\n');
  };

  let description = createDescription('full');
  // Fallback to compact table (no links) if description is too long.
  if (description.length > 4000) {
    description = createDescription('compact');
  }
  // Fallback to omitting table if still too long.
  if (description.length > 4000) {
    description = createDescription('minimal');
  }

  return description;
};

const COMPONENT_SYSTEMS = '1034653';
const COMPONENT_FLOPS = '1735976';

/**
 * Configuration for requesting repair for Browser devices.
 * Routes to FLOPS component by default.
 */
export const BrowserRepairConfig: RepairConfig<BrowserDeviceToRepair> = {
  componentId: COMPONENT_FLOPS,
  getTemplateId: (items) => (items.length === 1 ? '2107381' : '2161122'),
  generateTitle: generateBrowserTitle,
  generateDescription: (items) => {
    if (items.length === 1) {
      return generateBrowserIssueDescription(items[0], 'repair');
    }
    return generateBrowserBulkIssueDescription(items, 'repair');
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

/**
 * Configuration for requesting reinstall for Browser devices.
 * Routes to Systems component by default.
 */
export const BrowserReinstallConfig: RepairConfig<BrowserDeviceToRepair> = {
  componentId: COMPONENT_SYSTEMS,
  getTemplateId: (items) => (items.length === 1 ? '2107381' : '2161122'),
  generateTitle: (items) => {
    const title = generateBrowserTitle(items);
    return title.replace('[Repair]', '[Reinstall]');
  },
  generateDescription: (items) => {
    if (items.length === 1) {
      return generateBrowserIssueDescription(items[0], 'reinstall');
    }
    return generateBrowserBulkIssueDescription(items, 'reinstall');
  },
  hotlistIds: BrowserRepairConfig.hotlistIds,
};
