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
