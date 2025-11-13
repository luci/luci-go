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

import ConstructionIcon from '@mui/icons-material/Construction';
import DashboardIcon from '@mui/icons-material/Dashboard';
import DevicesIcon from '@mui/icons-material/Devices';
import DevicesOtherIcon from '@mui/icons-material/DevicesOther';
import LanIcon from '@mui/icons-material/Lan';
import TopicIcon from '@mui/icons-material/Topic';
import WarningIcon from '@mui/icons-material/Warning';
import React from 'react';

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import {
  generateDeviceListURL,
  generateRepairsURL,
  platformToURL,
} from '../constants/paths';

export interface SidebarPage {
  readonly label: string;
  readonly url: string;
  readonly icon: React.ReactNode;
  readonly external?: boolean;
}

export interface SidebarSection {
  readonly title: string;
  readonly pages: SidebarPage[];
}

export function generateSidebarSections(platform?: Platform): SidebarSection[] {
  return [
    generateLabHealthSection(platform),
    generateResourceRequestsSection(),
    generateOtherToolsSection(),
  ];
}

function generateLabHealthSection(platform?: Platform): SidebarSection {
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
      },
    ],
  };
}

function generateResourceRequestsSection(): SidebarSection {
  return {
    title: 'Resource Requests',
    pages: [
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
        label: 'Chrome Repairs',
        url: 'http://go/chrome-fleet-dashboard',
        icon: <DashboardIcon />,
        external: true,
      },
      {
        label: 'Nlyte',
        url: 'http://go/nlyte',
        icon: <LanIcon />,
        external: true,
      },
      {
        label: 'FLOPS Docs',
        url: 'http://go/flops-docs',
        icon: <TopicIcon />,
        external: true,
      },
      {
        label: 'Incidents (IRM)',
        url: 'http://go/fleet-irm',
        icon: <WarningIcon />,
        external: true,
      },
    ],
  };
}
