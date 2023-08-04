// Copyright 2023 The LUCI Authors.
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

import AccessTimeIcon from '@mui/icons-material/AccessTime';
import BuildIcon from '@mui/icons-material/Build';
import EngineeringIcon from '@mui/icons-material/Engineering';
import GrainTwoToneIcon from '@mui/icons-material/GrainTwoTone';
import HouseIcon from '@mui/icons-material/House';
import LineAxisIcon from '@mui/icons-material/LineAxis';
import LineStyleIcon from '@mui/icons-material/LineStyle';
import ScheduleIcon from '@mui/icons-material/Schedule';
import SearchIcon from '@mui/icons-material/Search';
import SpeedIcon from '@mui/icons-material/Speed';
import SpokeIcon from '@mui/icons-material/Spoke';
import TableViewIcon from '@mui/icons-material/TableView';
import VisibilityIcon from '@mui/icons-material/Visibility';
import React from 'react';

import { UiPage } from '@/common/constants';

export interface SidebarPage {
  readonly page: UiPage;
  readonly url: string;
  readonly icon: React.ReactNode;
  readonly external?: boolean;
}

export interface SidebarSection {
  readonly title: 'Builds' | 'Tests' | 'Monitoring' | 'Releases';
  readonly pages: SidebarPage[];
}

export function generateSidebarPages(project: string | undefined) {
  const sidebarSections: SidebarSection[] = [generateBuildsSection(project)];
  if (!project) {
    return sidebarSections;
  }

  sidebarSections.push(
    generateTestsSection(project),
    generateMonitoringSection(project)
  );

  if (project.startsWith('chrom')) {
    sidebarSections.push(generateReleasesSection(project));
  }

  return sidebarSections;
}

function generateBuildsSection(project: string | undefined): SidebarSection {
  const pages: SidebarPage[] = [];
  pages.push({
    page: UiPage.BuilderSearch,
    url: '/ui/builder-search',
    icon: <SearchIcon />,
  });
  if (project) {
    pages.push({
      page: UiPage.Builders,
      url: `/ui/p/${project}/builders`,
      icon: <BuildIcon />,
    });

    pages.push({
      page: UiPage.Scheduler,
      url: `https://luci-scheduler.appspot.com/jobs/${project}`,
      icon: <ScheduleIcon />,
      external: true,
    });


    if (project === 'chromium') {
      pages.push({
        page: UiPage.Bisection,
        url: `/ui/bisection`,
        icon: <GrainTwoToneIcon />,
      });
    }
  }

  return {
    title: 'Builds',
    pages,
  };
}

function generateTestsSection(project: string): SidebarSection {
  const pages: SidebarPage[] = [];
  pages.push(
    {
      page: UiPage.TestHistory,
      url: `/ui/p/${project}/test-search`,
      icon: <AccessTimeIcon />,
    },
    {
      page: UiPage.FailureClusters,
      url: `https://${CONFIGS.LUCI_ANALYSIS.HOST}/p/${project}/clusters`,
      icon: <SpokeIcon />,
      external: true,
    }
  );

  // Add ChromeOS specific Test tools.
  if (project.startsWith('chromeos')) {
    pages.push(
      {
        page: UiPage.Testhaus,
        url: `https://tests.chromeos.goog`,
        icon: <HouseIcon />,
        external: true,
      },
      {
        page: UiPage.Crosbolt,
        url: `https://healthmon.chromeos.goog/time_series`,
        icon: <SpeedIcon />,
        external: true,
      }
    );
  }
  return {
    title: 'Tests',
    pages,
  };
}

function generateMonitoringSection(project: string): SidebarSection {
  const pages: SidebarPage[] = [];
  pages.push({
    page: UiPage.Consoles,
    url: `/p/${project}`,
    icon: <TableViewIcon />,
    external: true,
  });

  appendSoM(project, pages);

  if (project.startsWith('chromeos')) {
    pages.push({
      page: UiPage.CQStatus,
      url: `http://go/cros-cq-status`,
      icon: <LineAxisIcon />,
      external: true,
    });
  }

  return {
    title: 'Monitoring',
    pages,
  };
}

export function appendSoM(project: string, pages: SidebarPage[]) {
  let soMURL = '';
  // Add SoM
  switch (true) {
    case project.startsWith('chromeos'): {
      soMURL = 'https://sheriff-o-matic.appspot.com/chromeos';
      break;
    }
    case project.startsWith('chromium') || project.startsWith('chrome'): {
      soMURL = 'https://sheriff-o-matic.appspot.com/chromium';
      break;
    }
    case project === 'fuchsia' || project === 'turquoise': {
      soMURL = 'https://sheriff-o-matic.appspot.com/fuchsia';
      break;
    }
  }

  if (soMURL !== '') {
    pages.push({
      page: UiPage.SoM,
      url: soMURL,
      icon: <EngineeringIcon />,
      external: true,
    });
  }
}

function generateReleasesSection(project: string): SidebarSection {
  const pages: SidebarPage[] = [];
  if (project.startsWith('chromeos')) {
    pages.push({
      page: UiPage.Goldeneye,
      url: 'https://cros-goldeneye.corp.google.com/',
      icon: <VisibilityIcon />,
      external: true,
    });
  } else if (project.startsWith('chrome') || project.startsWith('chromium')) {
    pages.push({
      page: UiPage.ChromiumDash,
      url: 'https://chromiumdash.appspot.com/',
      icon: <LineStyleIcon />,
      external: true,
    });
  }

  return {
    title: 'Releases',
    pages,
  };
}
