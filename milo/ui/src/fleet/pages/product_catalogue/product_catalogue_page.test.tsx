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

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { fakeUseSyncedSearchParams } from '@/fleet/testing_tools/mocks/fake_search_params';

import { ProductCataloguePage } from './product_catalogue_page';

jest.mock('@/fleet/hooks/prpc_clients');
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

describe('ProductCataloguePage', () => {
  it('should render successfully', async () => {
    fakeUseSyncedSearchParams();
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      ListProductCatalogEntries: {
        query: () => ({
          queryKey: ['ListProductCatalogEntries'],
          queryFn: async () => ({ entries: [] }),
        }),
      },
      GetProductCatalogFilterValues: {
        query: () => ({
          queryKey: ['GetProductCatalogFilterValues'],
          queryFn: async () => ({ productCatalogId: ['val1', 'val2'] }),
        }),
      },
    });

    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <SettingsProvider>
          <ShortcutProvider>
            <ProductCataloguePage />
          </ShortcutProvider>
        </SettingsProvider>
      </QueryClientProvider>,
    );

    // This test ensures the page can mount and render without throwing errors.
    // For example, it catches missing context providers, reference errors, and
    // infinite render loops (which was one example of a previous regression).
    expect(true).toBe(true);
  });
});
