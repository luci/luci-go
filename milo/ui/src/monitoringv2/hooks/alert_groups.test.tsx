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

import { renderHook } from '@testing-library/react';

import { useAlertGroupsClient } from '@/monitoringv2/hooks/prpc_clients';
import { MonitoringCtxForTest } from '@/monitoringv2/pages/monitoring_page/context';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { useAlertGroups } from './alert_groups';

jest.mock('@/monitoringv2/hooks/prpc_clients');

describe('useAlertGroups', () => {
  it('sanitizes tree name with underscores', () => {
    const listAlertGroupsQuerySpy = jest.fn().mockImplementation((opts) => ({
      queryKey: ['alert_groups', opts],
      queryFn: async () => ({ alertGroups: [] }),
    }));

    (useAlertGroupsClient as jest.Mock).mockReturnValue({
      ListAlertGroups: {
        query: listAlertGroupsQuerySpy,
      },
    });

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <FakeContextProvider>
        <MonitoringCtxForTest.Provider
          value={{
            tree: {
              name: 'tree_with@strange#characters.and.dots',
              display_name: 'Tree',
              project: 'proj',
              treeStatusName: 'status',
            },
            alerts: [],
            builderAlerts: [],
            stepAlerts: [],
            testAlerts: [],
            alertsLoading: false,
            alertsLoadingStatus: '',
          }}
        >
          {children}
        </MonitoringCtxForTest.Provider>
      </FakeContextProvider>
    );

    renderHook(() => useAlertGroups(), { wrapper });

    expect(listAlertGroupsQuerySpy).toHaveBeenCalledWith(
      expect.objectContaining({
        parent: 'rotations/tree-with-strange-characters.and.dots',
      }),
    );
  });
});
