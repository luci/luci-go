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

import DashboardIcon from '@mui/icons-material/Dashboard';
import DevicesIcon from '@mui/icons-material/Devices';
import EmojiNatureIcon from '@mui/icons-material/EmojiNature';
import LanIcon from '@mui/icons-material/Lan';
import TopicIcon from '@mui/icons-material/Topic';
import React from 'react';

import { generateDeviceListURL, CHROMEOS_PLATFORM } from '../constants/paths';

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

export function generateSidebarSections(): SidebarSection[] {
  return [
    generateFleetConsoleSection(),
    generateChromeOSSection(),
    generateChromeSection(),
    generateFleetManagementSection(),
  ];
}

function generateFleetConsoleSection(): SidebarSection {
  return {
    title: 'Fleet Console',
    pages: [
      {
        label: 'Metrics',
        url: '/ui/fleet/labs/metrics',
        icon: <DashboardIcon />,
      },
      {
        label: 'Devices',
        url: generateDeviceListURL(CHROMEOS_PLATFORM),
        icon: <DevicesIcon />,
      },
    ],
  };
}

function generateChromeOSSection(): SidebarSection {
  return {
    title: 'ChromeOS',
    pages: [
      {
        label: 'Swarming',
        url: 'https://chromeos-swarming.appspot.com/botlist',
        icon: <EmojiNatureIcon />,
        external: true,
      },
    ],
  };
}

function generateChromeSection(): SidebarSection {
  return {
    title: 'Chrome',
    pages: [
      {
        label: 'Fleet dashboard',
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
    ],
  };
}

function generateFleetManagementSection(): SidebarSection {
  return {
    title: 'Fleet Management',
    pages: [
      {
        label: 'FLOPS Docs',
        url: 'http://go/flops-docs',
        icon: <TopicIcon />,
        external: true,
      },
    ],
  };
}
