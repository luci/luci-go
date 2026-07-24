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
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import {
  SyncedSearchParamsProvider,
  useSyncedSearchParams,
} from '@/generic_libs/hooks/synced_search_params';

import { ProductCataloguePage } from './product_catalogue_page';

jest.mock('@/fleet/hooks/prpc_clients');
jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

const renderPage = (
  queryClient = new QueryClient(),
  initialEntries = ['/ui/fleet/catalog'],
) => {
  const entries = initialEntries.map((entry) => {
    const url = new URL(entry, 'http://localhost');
    if (!url.searchParams.has('view')) {
      url.searchParams.set('view', 'table');
    }
    return url.pathname + url.search + url.hash;
  });

  let searchParamsHook: ReturnType<typeof useSyncedSearchParams> | undefined;

  const TestComponent = () => {
    searchParamsHook = useSyncedSearchParams();
    return <ProductCataloguePage />;
  };

  const utils = render(
    <QueryClientProvider client={queryClient}>
      <SettingsProvider>
        <ShortcutProvider>
          <MemoryRouter initialEntries={entries}>
            <SyncedSearchParamsProvider>
              <TestComponent />
            </SyncedSearchParamsProvider>
          </MemoryRouter>
        </ShortcutProvider>
      </SettingsProvider>
    </QueryClientProvider>,
  );

  return {
    ...utils,
    getSearchParams: () => searchParamsHook![0],
    setSearchParams: (
      sp: URLSearchParams | ((prev: URLSearchParams) => URLSearchParams),
    ) => searchParamsHook![1](sp),
  };
};

describe('ProductCataloguePage', () => {
  it('should render successfully', async () => {
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

    renderPage(queryClient);

    // This test ensures the page can mount and render without throwing errors.
    // For example, it catches missing context providers, reference errors, and
    // infinite render loops (which was one example of a previous regression).
    expect(true).toBe(true);
  });

  it('should render R11N links correctly', async () => {
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

    const { findByText } = renderPage(queryClient);

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

    renderPage(queryClient);

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

  it('should render R11N in card view', async () => {
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
                r11n: ['r11n-val', 'r11n-val-2'],
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

    renderPage(queryClient);

    const switchBtn = await screen.findByRole('button', {
      name: /switch to card view/i,
    });
    fireEvent.click(switchBtn);

    expect(await screen.findByText('R11N')).toBeInTheDocument();
    expect(await screen.findByText('r11n-val, r11n-val-2')).toBeInTheDocument();
  });

  it('should render tabs, filter data/columns, and disable out-of-scope tabs', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      ListProductCatalogEntries: {
        query: (req?: { filter?: string }) => ({
          queryKey: ['ListProductCatalogEntries', req],
          queryFn: async () => {
            const all = [
              {
                productCatalogId: 'pc1',
                productName: 'Hardware Product',
                productType: 'hardware',
                numberOfDevicesPerRack: 0,
              },
              {
                productCatalogId: 'pc2',
                productName: 'Peripherals Product',
                productType: 'peripherals',
                numberOfDevicesPerRack: 10,
              },
            ];
            const entries =
              req?.filter && req.filter.includes('hardware')
                ? all.filter((e) => e.productType === 'hardware')
                : all;
            return { entries };
          },
        }),
      },
      GetProductCatalogFilterValues: {
        query: () => ({
          queryKey: ['GetProductCatalogFilterValues'],
          queryFn: async () => ({
            scopedProductType: [
              { value: 'hardware', inScope: true },
              { value: 'peripherals', inScope: false },
            ],
          }),
        }),
      },
    });

    const queryClient = new QueryClient();

    const { findByRole, findByText, queryByText } = renderPage(queryClient);

    // Should render the tabs (including prepended 'All')
    const allTab = await findByRole('tab', { name: 'All' });
    const hardwareTab = await findByRole('tab', { name: 'hardware' });
    const peripheralsTab = await findByRole('tab', { name: 'peripherals' });

    expect(allTab).toBeInTheDocument();
    expect(allTab).not.toBeDisabled();
    expect(allTab).toHaveAttribute('aria-selected', 'true'); // 'All' selected by default

    expect(hardwareTab).toBeInTheDocument();
    expect(hardwareTab).not.toBeDisabled();

    expect(peripheralsTab).toBeInTheDocument();
    expect(peripheralsTab).not.toBeDisabled();

    // Default 'All' view: both products and all columns should be displayed
    expect(await findByText('Hardware Product')).toBeInTheDocument();
    expect(await findByText('Peripherals Product')).toBeInTheDocument();
    expect(await findByText('Number of Devices Per Rack')).toBeInTheDocument();

    // Click on hardware tab
    fireEvent.click(hardwareTab);
    expect(hardwareTab).toHaveAttribute('aria-selected', 'true');

    // hardware tab view: only Hardware Product, and number of devices per rack should be visible
    expect(await findByText('Hardware Product')).toBeInTheDocument();
    expect(queryByText('Peripherals Product')).not.toBeInTheDocument();
    expect(queryByText('Number of Devices Per Rack')).toBeInTheDocument();
  });

  it('should render empty fallback message when there are no products', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    mockUseFleetConsoleClient.mockReturnValue({
      ListProductCatalogEntries: {
        query: () => ({
          queryKey: ['ListProductCatalogEntries'],
          queryFn: async () => ({
            entries: [],
          }),
        }),
      },
      GetProductCatalogFilterValues: {
        query: () => ({
          queryKey: ['GetProductCatalogFilterValues'],
          queryFn: async () => ({
            scopedProductType: [
              { value: 'hardware', inScope: false },
              { value: 'peripherals', inScope: false },
            ],
          }),
        }),
      },
    });

    const queryClient = new QueryClient();

    const { findByText } = renderPage(queryClient);

    expect(await findByText('No products found')).toBeInTheDocument();
  });

  it('should remove product_type filter when switching from All tab to another tab', async () => {
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
          queryFn: async () => ({
            scopedProductType: [
              { value: 'hardware', inScope: true },
              { value: 'peripherals', inScope: true },
            ],
          }),
        }),
      },
    });

    const queryClient = new QueryClient();
    const { findByRole, getSearchParams } = renderPage(queryClient, [
      '/ui/fleet/catalog?filters=product_type+%3D+%28%22hardware%22%29',
    ]);

    expect(getSearchParams().get('filters')).toContain('product_type');

    const hardwareTab = await findByRole('tab', { name: 'hardware' });
    fireEvent.click(hardwareTab);

    await waitFor(() => {
      expect(getSearchParams().get('filters') || '').not.toContain(
        'product_type',
      );
    });
  });

  it('should clear all filters when switching between tabs', async () => {
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
          queryFn: async () => ({
            scopedProductType: [
              { value: 'hardware', inScope: true },
              { value: 'peripherals', inScope: true },
            ],
          }),
        }),
      },
    });

    const queryClient = new QueryClient();
    const { findByRole, getSearchParams } = renderPage(queryClient, [
      '/ui/fleet/catalog?filters=gpn+%3D+%28%2212345%22%29',
    ]);

    expect(getSearchParams().get('filters')).toContain('gpn');

    const hardwareTab = await findByRole('tab', { name: 'hardware' });
    fireEvent.click(hardwareTab);

    await waitFor(() => {
      expect(getSearchParams().get('filters') || '').toBe('');
    });
  });

  it('should sync selected tab with the url parameter', async () => {
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
          queryFn: async () => ({
            scopedProductType: [
              { value: 'hardware', inScope: true },
              { value: 'peripherals', inScope: true },
            ],
          }),
        }),
      },
    });

    const queryClient = new QueryClient();
    const { findByRole, getSearchParams } = renderPage(queryClient);

    const hardwareTab = await findByRole('tab', { name: 'hardware' });
    fireEvent.click(hardwareTab);

    await waitFor(() => {
      expect(getSearchParams().get('tab')).toBe('hardware');
    });

    const allTab = await findByRole('tab', { name: 'All' });
    fireEvent.click(allTab);

    await waitFor(() => {
      expect(getSearchParams().get('tab')).toBe('All');
    });
  });

  it('should navigate to next page without freezing or resetting pageIndex', async () => {
    const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;

    const entries = Array.from({ length: 15 }, (_, i) => ({
      productCatalogId: `catalog-${i + 1}`,
      productName: `Product ${i + 1}`,
      gpn: `12345-${i + 1}`,
      descriptiveName: `Desc ${i + 1}`,
      resourceType: 'Type 1',
      fleetPlmStatus: 'Status 1',
      r11n: ['r11n-val'],
      numberOfDevicesPerRack: 10,
      unitCost: '100',
      productType: 'hardware',
    }));

    mockUseFleetConsoleClient.mockReturnValue({
      ListProductCatalogEntries: {
        query: () => ({
          queryKey: ['ListProductCatalogEntries'],
          queryFn: async () => ({ entries }),
        }),
      },
      GetProductCatalogFilterValues: {
        query: () => ({
          queryKey: ['GetProductCatalogFilterValues'],
          queryFn: async () => ({
            scopedProductType: [{ value: 'hardware', inScope: true }],
          }),
        }),
      },
    });

    const queryClient = new QueryClient();
    renderPage(queryClient, ['/ui/fleet/catalog?pageSize=10']);

    expect(await screen.findByText('Product 1')).toBeInTheDocument();
    expect(screen.queryByText('Product 12')).not.toBeInTheDocument();

    const nextPageBtn = screen.getByRole('button', { name: /next page/i });
    fireEvent.click(nextPageBtn);

    await waitFor(() => {
      expect(screen.getByText('Product 12')).toBeInTheDocument();
      expect(screen.queryByText('Product 1')).not.toBeInTheDocument();
    });
  }, 15000);
});
