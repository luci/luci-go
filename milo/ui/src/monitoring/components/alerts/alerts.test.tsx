// Copyright 2024 The LUCI Authors.
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

import { render, screen } from '@testing-library/react';

import { MonitoringCtxForTest } from '@/monitoring/pages/monitoring_page/context';
import { configuredTrees } from '@/monitoring/util/config';
import { Bug } from '@/monitoring/util/server_json';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { Alerts } from './alerts';

const hotlistBug: Bug = {
  summary: 'Hotlist Bug',
  labels: [],
  link: 'https://b/1234',
  number: '1234',
  priority: 1,
  status: 'Fixed',
};

describe('<Alerts />', () => {
  it('displays filter and alert groups', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?alerts_tab=untriaged'],
        }}
      >
        <MonitoringCtxForTest.Provider
          value={{
            alerts: [],
            bugs: [],
            tree: configuredTrees[0],
          }}
        >
          <Alerts />
        </MonitoringCtxForTest.Provider>
      </FakeContextProvider>,
    );
    expect(screen.getByRole('searchbox')).toBeInTheDocument();
    expect(
      screen.getByText('Untriaged Consistent Failures'),
    ).toBeInTheDocument();
    expect(screen.getByText('Untriaged New Failures')).toBeInTheDocument();
  });

  it('displays no bugs mesage', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?alerts_tab=bugs'],
        }}
      >
        <MonitoringCtxForTest.Provider
          value={{
            alerts: [],
            bugs: [],
            tree: configuredTrees[0],
          }}
        >
          <Alerts />
        </MonitoringCtxForTest.Provider>
      </FakeContextProvider>,
    );
    expect(
      screen.getByText('There are currently no alerts associated with bugs', {
        exact: false,
      }),
    ).toBeInTheDocument();
  });

  it('displays a group for a bug in the hotlist when there are no alerts', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?alerts_tab=bugs'],
        }}
      >
        <MonitoringCtxForTest.Provider
          value={{
            alerts: [],
            bugs: [hotlistBug],
            tree: configuredTrees[0],
          }}
        >
          <Alerts />
        </MonitoringCtxForTest.Provider>
      </FakeContextProvider>,
    );
    expect(screen.getByText('Hotlist Bug')).toBeInTheDocument();
  });

  it('sets filter when icon clicked', async () => {
    render(
      <FakeContextProvider
        routerOptions={{
          initialEntries: ['/?q=linux-rel&alerts_tab=untriaged'],
        }}
        mountedPath="/"
      >
        <MonitoringCtxForTest.Provider
          value={{
            alerts: [],
            bugs: [hotlistBug],
            tree: configuredTrees[0],
          }}
        >
          <Alerts />
        </MonitoringCtxForTest.Provider>
      </FakeContextProvider>,
    );
    expect(screen.getByRole('searchbox')).toHaveValue('linux-rel');
  });
});
