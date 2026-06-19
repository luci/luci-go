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
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

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
            <MemoryRouter>
              <ProductCataloguePage />
            </MemoryRouter>
          </ShortcutProvider>
        </SettingsProvider>
      </QueryClientProvider>,
    );

    // This test ensures the page can mount and render without throwing errors.
    // For example, it catches missing context providers, reference errors, and
    // infinite render loops (which was one example of a previous regression).
    expect(true).toBe(true);
  });

  it('should render R11N links correctly', async () => {
    fakeUseSyncedSearchParams();
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      ListProductCatalogEntries: {
        query: () => ({
          queryKey: ['ListProductCatalogEntries'],
          queryFn: async () => ({
            entries: [
              {
                productCatalogId: 'catalog-1',
                productName: 'Product 1',
                gpn: '12345',
                descriptiveName: 'Desc 1',
                resourceType: 'Type 1',
                fleetPlmStatus: 'Status 1',
                r11n: ['r11n-val', 'TBD'],
                numberOfDevicesPerRack: 10,
                unitCost: '100',
                productType: 'Type A',
              },
            ],
          }),
        }),
      },
      GetProductCatalogFilterValues: {
        query: () => ({
          queryKey: ['GetProductCatalogFilterValues'],
          queryFn: async () => ({ productCatalogId: [] }),
        }),
      },
    });

    const queryClient = new QueryClient();

    const { findByText } = render(
      <QueryClientProvider client={queryClient}>
        <SettingsProvider>
          <ShortcutProvider>
            <MemoryRouter>
              <ProductCataloguePage />
            </MemoryRouter>
          </ShortcutProvider>
        </SettingsProvider>
      </QueryClientProvider>,
    );

    const link1 = await findByText('r11n-val');
    expect(link1).toBeInTheDocument();
    expect(link1.tagName).toBe('A');
    expect(link1).toHaveAttribute('href', 'http://go/ngp-npi/r11n/r11n-val');

    const link2 = await findByText('TBD');
    expect(link2).toBeInTheDocument();
    expect(link2.tagName).toBe('A');
    expect(link2).toHaveAttribute('href', 'http://go/ngp-npi/r11n/tbd');
  });

  it('should sort client-side by R11N correctly', async () => {
    fakeUseSyncedSearchParams();
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      ListProductCatalogEntries: {
        query: () => ({
          queryKey: ['ListProductCatalogEntries'],
          queryFn: async () => ({
            entries: [
              {
                productCatalogId: 'catalog-1',
                productName: 'Product B',
                gpn: '12345',
                descriptiveName: 'Desc 1',
                resourceType: 'Type 1',
                fleetPlmStatus: 'Status 1',
                r11n: ['B-val'],
                numberOfDevicesPerRack: 10,
                unitCost: '100',
                productType: 'Type A',
              },
              {
                productCatalogId: 'catalog-2',
                productName: 'Product A',
                gpn: '12346',
                descriptiveName: 'Desc 2',
                resourceType: 'Type 2',
                fleetPlmStatus: 'Status 2',
                r11n: ['A-val'],
                numberOfDevicesPerRack: 10,
                unitCost: '100',
                productType: 'Type A',
              },
            ],
          }),
        }),
      },
      GetProductCatalogFilterValues: {
        query: () => ({
          queryKey: ['GetProductCatalogFilterValues'],
          queryFn: async () => ({ productCatalogId: [] }),
        }),
      },
    });

    const queryClient = new QueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <SettingsProvider>
          <ShortcutProvider>
            <MemoryRouter>
              <ProductCataloguePage />
            </MemoryRouter>
          </ShortcutProvider>
        </SettingsProvider>
      </QueryClientProvider>,
    );

    // Verify initial rendering order: B-val then A-val
    await screen.findByText('B-val');
    await screen.findByText('A-val');
    const links = screen.getAllByRole('link');
    const r11nLinks = links.filter((l) =>
      l.getAttribute('href')?.includes('/r11n/'),
    );
    expect(r11nLinks).toHaveLength(2);
    expect(r11nLinks[0]).toHaveTextContent('B-val');
    expect(r11nLinks[1]).toHaveTextContent('A-val');

    // Find the header for R11N and click it to sort
    const r11nSortButtons = await screen.findAllByLabelText(/Sort by R11N/i);
    const sortBtn = r11nSortButtons.find((b) =>
      b.classList.contains('MuiTableSortLabel-root'),
    );
    fireEvent.click(sortBtn!);
    fireEvent.click(sortBtn!);

    // Verify sorted order: A-val then B-val
    await waitFor(() => {
      const sortedLinks = screen.getAllByRole('link');
      const sortedR11nLinks = sortedLinks.filter((l) =>
        l.getAttribute('href')?.includes('/r11n/'),
      );
      expect(sortedR11nLinks).toHaveLength(2);
      expect(sortedR11nLinks[0]).toHaveTextContent('A-val');
      expect(sortedR11nLinks[1]).toHaveTextContent('B-val');
    });
  });
});
