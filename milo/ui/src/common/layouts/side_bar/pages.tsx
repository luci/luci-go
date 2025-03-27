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

import { PlagiarismOutlined, UTurnLeft } from '@mui/icons-material';
import AccessTimeIcon from '@mui/icons-material/AccessTime';
import BuildIcon from '@mui/icons-material/Build';
import EngineeringIcon from '@mui/icons-material/Engineering';
import GrainTwoToneIcon from '@mui/icons-material/GrainTwoTone';
import GroupsIcon from '@mui/icons-material/Groups';
import HouseIcon from '@mui/icons-material/House';
import LineAxisIcon from '@mui/icons-material/LineAxis';
import LineStyleIcon from '@mui/icons-material/LineStyle';
import RuleIcon from '@mui/icons-material/Rule';
import ScheduleIcon from '@mui/icons-material/Schedule';
import SearchIcon from '@mui/icons-material/Search';
import SpeedIcon from '@mui/icons-material/Speed';
import SpokeIcon from '@mui/icons-material/Spoke';
import TableViewIcon from '@mui/icons-material/TableView';
import TrafficIcon from '@mui/icons-material/Traffic';
import VisibilityIcon from '@mui/icons-material/Visibility';
import React from 'react';

import { UiPage } from '@/common/constants/view';
import { getProjectURLPath } from '@/common/tools/url_utils';

export interface SidebarPage {
  readonly page: UiPage;
  readonly url: string;
  readonly icon: React.ReactNode;
  readonly external?: boolean;
}

export interface SidebarSection {
  readonly title:
    | 'Builds'
    | 'Tests'
    | 'Monitoring'
    | 'Releases'
    | 'Admin'
    | 'Test Analysis';
  readonly pages: SidebarPage[];
}

export function generateSidebarSections(
  project: string | undefined,
  treeNames: string[] | undefined,
  email: string | undefined,
) {
  return (
    [
      generateBuildsSection(project),
      generateTestsSection(project),
      generateTestAnalysisSection(project),
      generateMonitoringSection(project, treeNames),
      generateReleasesSection(project),
      generateAuthServiceSection(email),
    ]
      // Remove empty sections.
      .filter((sec) => sec.pages.length)
  );
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
      page: UiPage.BuilderGroups,
      url: getProjectURLPath(project),
      icon: <TableViewIcon />,
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
        url: `/ui/p/${project}/bisection`,
        icon: <GrainTwoToneIcon />,
      });
    }
  }

  return {
    title: 'Builds',
    pages,
  };
}

function generateTestsSection(project: string | undefined): SidebarSection {
  const pages: SidebarPage[] = [];

  if (project) {
    pages.push(
      {
        page: UiPage.TestHistory,
        url: `/ui/p/${project}/test-search`,
        icon: <AccessTimeIcon />,
      },
      {
        page: UiPage.RecentRegressions,
        url: `/ui/labs/p/${project}/regressions`,
        icon: <UTurnLeft />,
      },
      {
        page: UiPage.LogSearch,
        url: `/ui/labs/p/${project}/log-search`,
        icon: <PlagiarismOutlined />,
      },
    );
  }

  // Add ChromeOS specific Test tools.
  if (project?.startsWith('chromeos')) {
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
      },
    );
  }

  return {
    title: 'Tests',
    pages,
  };
}

function generateMonitoringSection(
  project: string | undefined,
  treeNames: string[] | undefined,
): SidebarSection {
  const pages: SidebarPage[] = [];

  const somProject = getSomProject(project);
  if (somProject) {
    pages.push({
      page: UiPage.SoM,
      url: `https://sheriff-o-matic.appspot.com/${somProject}`,
      icon: <EngineeringIcon />,
      external: true,
    });
  }

  if (project?.startsWith('chromeos')) {
    pages.push({
      page: UiPage.CQStatus,
      url: `http://go/cros-cq-status`,
      icon: <LineAxisIcon />,
      external: true,
    });
  }

  // We only display tree status if there is exactly one tree for the project
  if (treeNames?.length === 1 && project) {
    // TreeName is in the form "trees/<tree_id>".
    const treeID = treeNames[0].substring(6);
    pages.push({
      page: UiPage.TreeStatus,
      url: `/ui/tree-status/${treeID}?project=${project}`,
      icon: <TrafficIcon />,
    });
  }

  return {
    title: 'Monitoring',
    pages,
  };
}

export function getSomProject(project: string | undefined): string | null {
  if (!project) {
    return null;
  }

  if (project.startsWith('chromeos')) {
    return 'chromeos';
  }
  if (project.startsWith('chromium') || project.startsWith('chrome')) {
    return 'chromium';
  }
  if (project === 'fuchsia' || project === 'turquoise') {
    return 'fuchsia';
  }

  return null;
}

function generateReleasesSection(project: string | undefined): SidebarSection {
  const pages: SidebarPage[] = [];

  if (project?.startsWith('chromeos')) {
    pages.push({
      page: UiPage.Goldeneye,
      url: 'https://cros-goldeneye.corp.google.com/',
      icon: <VisibilityIcon />,
      external: true,
    });
  } else if (project?.startsWith('chrome') || project?.startsWith('chromium')) {
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

function generateAuthServiceSection(email: string | undefined): SidebarSection {
  const pages: SidebarPage[] = [];
  // External users cannot see auth service, so only link it for Googlers.
  const showAuthService = /@google\.com$/.test(email || '');
  if (showAuthService) {
    pages.push(
      {
        page: UiPage.AuthServiceGroups,
        url: '/ui/auth/groups',
        icon: <GroupsIcon />,
      },
      {
        page: UiPage.AuthServiceLookup,
        url: '/ui/auth/lookup',
        icon: <SearchIcon />,
      },
    );
  }
  return {
    title: 'Admin',
    pages,
  };
}

function generateTestAnalysisSection(
  project: string | undefined,
): SidebarSection {
  const pages: SidebarPage[] = [];
  if (project) {
    pages.push({
      page: UiPage.Clusters,
      url: `/ui/tests/p/${project}/clusters`,
      icon: <SpokeIcon />,
    });
    pages.push({
      page: UiPage.Rules,
      url: `/ui/tests/p/${project}/rules`,
      icon: <RuleIcon />,
    });
  }

  return {
    title: 'Test Analysis',
    pages,
  };
}
