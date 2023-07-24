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
import SearchIcon from '@mui/icons-material/Search';
import SpeedIcon from '@mui/icons-material/Speed';
import SpokeIcon from '@mui/icons-material/Spoke';
import TableViewIcon from '@mui/icons-material/TableView';
import VisibilityIcon from '@mui/icons-material/Visibility';

import { UiPage } from '@/common/constants';

import {
  SidebarPage,
  SidebarSection,
  appendSoM,
  generateSidebarPages,
} from './pages';

describe('pages', () => {
  it('should generate list with only builders search if there is no project', () => {
    const sidebarItems = generateSidebarPages(undefined);
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

  it('should generate basic items for all projects', () => {
    const sidebarItems = generateSidebarPages('projecttest');
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
            url: `https://${CONFIGS.LUCI_ANALYSIS.HOST}/p/projecttest/clusters`,
            icon: <SpokeIcon />,
            external: true,
          },
        ],
      },
      {
        title: 'Monitoring',
        pages: [
          {
            page: UiPage.Consoles,
            url: '/p/projecttest',
            icon: <TableViewIcon />,
            external: true,
          },
        ],
      },
    ]);
  });

  it('should generate correct links for chromium', () => {
    const sidebarItems = generateSidebarPages('chromium');
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
            page: UiPage.Bisection,
            url: `/ui/bisection`,
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
            url: `https://${CONFIGS.LUCI_ANALYSIS.HOST}/p/chromium/clusters`,
            icon: <SpokeIcon />,
            external: true,
          },
        ],
      },
      {
        title: 'Monitoring',
        pages: [
          {
            page: UiPage.Consoles,
            url: '/p/chromium',
            icon: <TableViewIcon />,
            external: true,
          },
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
    const sidebarItems = generateSidebarPages('chromeos');
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
            url: `https://${CONFIGS.LUCI_ANALYSIS.HOST}/p/chromeos/clusters`,
            icon: <SpokeIcon />,
            external: true,
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
            page: UiPage.Consoles,
            url: '/p/chromeos',
            icon: <TableViewIcon />,
            external: true,
          },
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
  describe('appendSoM', () => {
    it('given unknown project, should not generate item', () => {
      const pages: SidebarPage[] = [];
      appendSoM('unknown', pages);
      expect(pages.length).toBe(0);
    });

    it.each([
      ['chromeos', 'https://sheriff-o-matic.appspot.com/chromeos'],
      ['chrome', 'https://sheriff-o-matic.appspot.com/chromium'],
      ['chromium', 'https://sheriff-o-matic.appspot.com/chromium'],
      ['fuchsia', 'https://sheriff-o-matic.appspot.com/fuchsia'],
      ['turquoise', 'https://sheriff-o-matic.appspot.com/fuchsia'],
    ])(
      'given %p project, should generate matching link',
      (project: string, url: string) => {
        const pages: SidebarPage[] = [];
        appendSoM(project, pages);
        expect(pages.length).not.toBe(0);
        expect(pages[0].url).toEqual(url);
      }
    );
  });
});
