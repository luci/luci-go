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
import ScheduleIcon from '@mui/icons-material/Schedule';
import SearchIcon from '@mui/icons-material/Search';
import SpeedIcon from '@mui/icons-material/Speed';
import SpokeIcon from '@mui/icons-material/Spoke';
import TableViewIcon from '@mui/icons-material/TableView';
import VisibilityIcon from '@mui/icons-material/Visibility';

import { UiPage } from '@/common/constants/view';

import {
  SidebarSection,
  generateSidebarSections,
  getSomProject,
} from './pages';

describe('generateSidebarSections', () => {
  it('should generate list with only builders search if there is no project', () => {
    const sidebarItems = generateSidebarSections(undefined, undefined, undefined);
    expect(sidebarItems).toEqual<SidebarSection[]>([
      {
        title: `Builds`,
        pages: [
          {
            page: UiPage.BuilderSearch,
            url: '/ui/builder-search',
            icon: <SearchIcon />,
          },
        ],
      },
    ]);
  });

  it('should generate groups link when googler is logged in', () => {
    const sidebarItems = generateSidebarSections(undefined, undefined, 'emailtest@google.com');
    expect(sidebarItems).toEqual<SidebarSection[]>([
      {
        title: `Builds`,
        pages: [
          {
            page: UiPage.BuilderSearch,
            url: '/ui/builder-search',
            icon: <SearchIcon />,
          },
        ],
      },
      {
        title: `Admin`,
        pages: [
          {
            page: UiPage.AuthService,
            url: '/ui/auth/groups',
            icon: <GroupsIcon />,
          },
        ],
      },
    ]);
  });

  it('should generate basic items for all projects', () => {
    const sidebarItems = generateSidebarSections('projecttest', undefined, undefined);
    expect(sidebarItems).toEqual<SidebarSection[]>([
      {
        title: `Builds`,
        pages: [
          {
            page: UiPage.BuilderSearch,
            url: '/ui/builder-search',
            icon: <SearchIcon />,
          },
          {
            page: UiPage.Builders,
            url: `/ui/p/projecttest/builders`,
            icon: <BuildIcon />,
          },
          {
            page: UiPage.BuilderGroups,
            url: '/ui/p/projecttest',
            icon: <TableViewIcon />,
          },
          {
            page: UiPage.Scheduler,
            url: `https://luci-scheduler.appspot.com/jobs/projecttest`,
            icon: <ScheduleIcon />,
            external: true,
          },
        ],
      },
      {
        title: `Tests`,
        pages: [
          {
            page: UiPage.TestHistory,
            url: `/ui/p/projecttest/test-search`,
            icon: <AccessTimeIcon />,
          },
          {
            page: UiPage.FailureClusters,
            url: `https://${SETTINGS.luciAnalysis.uiHost || SETTINGS.luciAnalysis.host}/p/projecttest/clusters`,
            icon: <SpokeIcon />,
            external: true,
          },
          {
            page: UiPage.RecentRegressions,
            url: `/ui/labs/p/projecttest/regressions`,
            icon: <UTurnLeft />,
          },
          {
            page: UiPage.LogSearch,
            url: `/ui/labs/p/projecttest/log-search`,
            icon: <PlagiarismOutlined />,
          },
        ],
      },
    ]);
  });

  it('should generate correct links for chromium', () => {
    const sidebarItems = generateSidebarSections('chromium', undefined, undefined);
    expect(sidebarItems).toEqual<SidebarSection[]>([
      {
        title: `Builds`,
        pages: [
          {
            page: UiPage.BuilderSearch,
            url: '/ui/builder-search',
            icon: <SearchIcon />,
          },
          {
            page: UiPage.Builders,
            url: `/ui/p/chromium/builders`,
            icon: <BuildIcon />,
          },
          {
            page: UiPage.BuilderGroups,
            url: '/ui/p/chromium',
            icon: <TableViewIcon />,
          },
          {
            page: UiPage.Scheduler,
            url: `https://luci-scheduler.appspot.com/jobs/chromium`,
            icon: <ScheduleIcon />,
            external: true,
          },
          {
            page: UiPage.Bisection,
            url: `/ui/p/chromium/bisection`,
            icon: <GrainTwoToneIcon />,
          },
        ],
      },
      {
        title: `Tests`,
        pages: [
          {
            page: UiPage.TestHistory,
            url: `/ui/p/chromium/test-search`,
            icon: <AccessTimeIcon />,
          },
          {
            page: UiPage.FailureClusters,
            url: `https://${SETTINGS.luciAnalysis.uiHost || SETTINGS.luciAnalysis.host}/p/chromium/clusters`,
            icon: <SpokeIcon />,
            external: true,
          },
          {
            page: UiPage.RecentRegressions,
            url: `/ui/labs/p/chromium/regressions`,
            icon: <UTurnLeft />,
          },
          {
            page: UiPage.LogSearch,
            url: `/ui/labs/p/chromium/log-search`,
            icon: <PlagiarismOutlined />,
          },
        ],
      },
      {
        title: 'Monitoring',
        pages: [
          {
            page: UiPage.SoM,
            url: 'https://sheriff-o-matic.appspot.com/chromium',
            icon: <EngineeringIcon />,
            external: true,
          },
        ],
      },
      {
        title: 'Releases',
        pages: [
          {
            page: UiPage.ChromiumDash,
            url: 'https://chromiumdash.appspot.com/',
            icon: <LineStyleIcon />,
            external: true,
          },
        ],
      },
    ]);
  });

  it('should generate correct links for chromeos', () => {
    const sidebarItems = generateSidebarSections('chromeos', undefined, undefined);
    expect(sidebarItems).toEqual<SidebarSection[]>([
      {
        title: `Builds`,
        pages: [
          {
            page: UiPage.BuilderSearch,
            url: '/ui/builder-search',
            icon: <SearchIcon />,
          },
          {
            page: UiPage.Builders,
            url: `/ui/p/chromeos/builders`,
            icon: <BuildIcon />,
          },
          {
            page: UiPage.BuilderGroups,
            url: '/ui/p/chromeos',
            icon: <TableViewIcon />,
          },
          {
            page: UiPage.Scheduler,
            url: `https://luci-scheduler.appspot.com/jobs/chromeos`,
            icon: <ScheduleIcon />,
            external: true,
          },
        ],
      },
      {
        title: `Tests`,
        pages: [
          {
            page: UiPage.TestHistory,
            url: `/ui/p/chromeos/test-search`,
            icon: <AccessTimeIcon />,
          },
          {
            page: UiPage.FailureClusters,
            url: `https://${SETTINGS.luciAnalysis.uiHost || SETTINGS.luciAnalysis.host}/p/chromeos/clusters`,
            icon: <SpokeIcon />,
            external: true,
          },
          {
            page: UiPage.RecentRegressions,
            url: `/ui/labs/p/chromeos/regressions`,
            icon: <UTurnLeft />,
          },
          {
            page: UiPage.LogSearch,
            url: `/ui/labs/p/chromeos/log-search`,
            icon: <PlagiarismOutlined />,
          },
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
        ],
      },
      {
        title: 'Monitoring',
        pages: [
          {
            page: UiPage.SoM,
            url: 'https://sheriff-o-matic.appspot.com/chromeos',
            icon: <EngineeringIcon />,
            external: true,
          },
          {
            page: UiPage.CQStatus,
            url: `http://go/cros-cq-status`,
            icon: <LineAxisIcon />,
            external: true,
          },
        ],
      },
      {
        title: 'Releases',
        pages: [
          {
            page: UiPage.Goldeneye,
            url: 'https://cros-goldeneye.corp.google.com/',
            icon: <VisibilityIcon />,
            external: true,
          },
        ],
      },
    ]);
  });
});

describe('getSomProject', () => {
  it('given unknown project, should not generate item', () => {
    expect(getSomProject('unknown')).toBeNull();
  });

  it.each([
    ['chromeos', 'chromeos'],
    ['chrome', 'chromium'],
    ['chromium', 'chromium'],
    ['fuchsia', 'fuchsia'],
    ['turquoise', 'fuchsia'],
  ])(
    'given %p project, should generate matching link',
    (project: string, somProject: string) => {
      expect(getSomProject(project)).toStrictEqual(somProject);
    },
  );
});
