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

import ArticleIcon from '@mui/icons-material/Article';
import ConstructionIcon from '@mui/icons-material/Construction';
import DashboardIcon from '@mui/icons-material/Dashboard';
import DevicesIcon from '@mui/icons-material/Devices';
import DevicesOtherIcon from '@mui/icons-material/DevicesOther';
import HomeIcon from '@mui/icons-material/Home';
import LanIcon from '@mui/icons-material/Lan';
import TaskIcon from '@mui/icons-material/Task';
import TopicIcon from '@mui/icons-material/Topic';
import WarningIcon from '@mui/icons-material/Warning';
import React from 'react';

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { getFeatureFlag } from '../config/features';
import {
  generateAdminTasksURL,
  generateDeviceListURL,
  generateRepairsURL,
  platformToURL,
} from '../constants/paths';

export interface SidebarPage {
  readonly label: string;
  readonly url: string;
  readonly icon: React.ReactNode;
  readonly external?: boolean;
  readonly disabled?: boolean;
  readonly tooltip?: string;
  readonly badgeCount?: number;
  readonly badgeAriaLabel?: string;
}

export interface SidebarSection {
  readonly title: string;
  readonly pages: SidebarPage[];
}

export function generateSidebarSections(
  platform?: Platform,
  pendingAdminTasksCount?: number,
): SidebarSection[] {
  return [
    generateHomeSection(),
    generateLabHealthSection(platform, pendingAdminTasksCount),
    generateResourceRequestsSection(),
    generateOtherToolsSection(),
  ];
}

function generateHomeSection(): SidebarSection {
  return {
    title: '',
    pages: [
      {
        label: 'Home',
        url: '/ui/fleet/',
        icon: <HomeIcon />,
      },
    ],
  };
}

function generateLabHealthSection(
  platform?: Platform,
  pendingAdminTasksCount?: number,
): SidebarSection {
  const isRepairsEnabled = !platform || platform === Platform.ANDROID;
  const isAdminTasksEnabled = !platform || platform === Platform.CHROMEOS;
  return {
    title: 'Lab Health',
    pages: [
      {
        label: 'Metrics',
        url: '/ui/fleet/labs/metrics',
        icon: <DashboardIcon />,
      },
      {
        label: 'Devices',
        url: generateDeviceListURL(
          platformToURL(platform || Platform.CHROMEOS),
        ),
        icon: <DevicesIcon />,
      },
      {
        label: 'Repairs',
        url: generateRepairsURL(platformToURL(platform || Platform.ANDROID)),
        icon: <ConstructionIcon />,
        disabled: !isRepairsEnabled,
        tooltip: !isRepairsEnabled
          ? `Repairs for ${getPlatformName(platform)} are not available yet`
          : undefined,
      },
      ...(isAdminTasksEnabled
        ? [
            {
              label: 'Admin tasks',
              url: generateAdminTasksURL(
                platformToURL(platform || Platform.CHROMEOS),
              ),
              icon: <TaskIcon />,
              badgeCount: pendingAdminTasksCount,
              badgeAriaLabel: `${pendingAdminTasksCount ?? 0} pending admin tasks`,
            },
          ]
        : []),
    ],
  };
}

function getPlatformName(platform?: Platform): string {
  switch (platform) {
    case Platform.CHROMEOS:
      return 'ChromeOS';
    case Platform.ANDROID:
      return 'Android';
    case Platform.CHROMIUM:
      return 'Chrome Browser';
    default:
      return 'this platform';
  }
}

function generateResourceRequestsSection(): SidebarSection {
  return {
    title: 'Resource Requests',
    pages: [
      ...(getFeatureFlag('ProductCatalogListPage')
        ? [
            {
              label: 'Product Catalog',
              url: '/ui/fleet/labs/catalog',
              icon: <ArticleIcon />,
            },
          ]
        : []),
      {
        label: 'Requester Insights',
        url: '/ui/fleet/labs/requests',
        icon: <DevicesOtherIcon />,
      },
      {
        label: 'Planner Insights',
        url: '/ui/fleet/labs/planners',
        icon: <DashboardIcon />,
      },
    ],
  };
}

function generateOtherToolsSection(): SidebarSection {
  return {
    title: 'Other Tools',
    pages: [
      {
        label: 'Incidents (IRM)',
        url: 'http://go/fleetops-irm',
        icon: <WarningIcon />,
        external: true,
      },
      {
        label: 'Fleet Team',
        url: 'http://go/fleet',
        icon: <TopicIcon />,
        external: true,
      },
      {
        label: 'Nlyte Fleet Space Inventory',
        url: 'https://data.corp.google.com/sites/fo_quick_links/nlyte_fleet_space_inventory/',
        icon: <LanIcon />,
        external: true,
      },
    ],
  };
}
