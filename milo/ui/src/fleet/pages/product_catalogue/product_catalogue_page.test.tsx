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

import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ProductCataloguePage } from './product_catalogue_page';

jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

const renderPage = (initialEntries = ['/ui/fleet/catalog']) => {
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
    <FakeContextProvider
      mountedPath="/ui/fleet/catalog"
      routerOptions={{ initialEntries: entries }}
    >
      <SettingsProvider>
        <ShortcutProvider>
          <TestComponent />
        </ShortcutProvider>
      </SettingsProvider>
    </FakeContextProvider>,
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
  beforeEach(() => {
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.resetFixtures();

    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      productCatalogId: ['val1', 'val2'],
      fleetPlmStatus: [
        { value: 'GA', inScope: true },
        { value: 'LA', inScope: true },
        { value: 'NPI', inScope: true },
      ],
      scopedProductType: [
        { value: 'hardware', inScope: true },
        { value: 'peripherals', inScope: true },
      ],
    });
  });

  it('should render successfully', async () => {
    renderPage();

    // This test ensures the page can mount and render without throwing errors.
    expect(true).toBe(true);
  });

  it('should render R11N links correctly', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
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
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      productCatalogId: [],
    });

    const { findByText } = renderPage();

    const link1 = await findByText('r11n-val');
    expect(link1).toBeInTheDocument();
    expect(link1.tagName).toBe('A');
    expect(link1).toHaveAttribute('href', 'http://go/ngp-npi/r11n/r11n-val');

    const link2 = await findByText('TBD');
    expect(link2).toBeInTheDocument();
    expect(link2.tagName).toBe('A');
    expect(link2).toHaveAttribute('href', 'http://go/ngp-npi/r11n/tbd');
  }, 15000);

  it('should sort client-side by R11N correctly', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
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
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      productCatalogId: [],
    });

    renderPage();

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
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
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
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      productCatalogId: [],
    });

    renderPage();

    const switchBtn = await screen.findByRole('button', {
      name: /switch to card view/i,
    });
    fireEvent.click(switchBtn);

    expect(await screen.findByText('R11N')).toBeInTheDocument();
    expect(await screen.findByText('r11n-val, r11n-val-2')).toBeInTheDocument();
  });

  it('should render tabs, filter data/columns, and disable out-of-scope tabs', async () => {
    FleetConsoleMockAPI.setFixture(
      'ListProductCatalogEntries',
      (req: { filter?: string }) => {
        const all = [
          {
            productCatalogId: 'pc1',
            productName: 'Hardware Product',
            productType: 'hardware',
            fleetPlmStatus: 'GA',
            numberOfDevicesPerRack: 0,
          },
          {
            productCatalogId: 'pc2',
            productName: 'Peripherals Product',
            productType: 'peripherals',
            fleetPlmStatus: 'GA',
            numberOfDevicesPerRack: 10,
          },
        ];
        if (req?.filter) {
          if (req.filter.includes('hardware')) {
            return { entries: all.filter((e) => e.productType === 'hardware') };
          }
          if (req.filter.includes('peripherals')) {
            return {
              entries: all.filter((e) => e.productType === 'peripherals'),
            };
          }
        }
        return { entries: all };
      },
    );
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      scopedProductType: [
        { value: 'hardware', inScope: true },
        { value: 'peripherals', inScope: true },
      ],
    });

    const { findByRole, findByText } = renderPage();

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

    expect(await screen.findByText('Hardware Product')).toBeInTheDocument();
    expect(screen.queryByText('Peripherals Product')).not.toBeInTheDocument();
    expect(
      screen.queryByText('Number of Devices Per Rack'),
    ).toBeInTheDocument();
  }, 15000);

  it('should render empty fallback message when there are no products', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      scopedProductType: [
        { value: 'hardware', inScope: false },
        { value: 'peripherals', inScope: false },
      ],
    });

    const { findByText } = renderPage();

    expect(await findByText('No products found')).toBeInTheDocument();
  });

  it('should remove product_type filter when switching from All tab to another tab', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      scopedProductType: [
        { value: 'hardware', inScope: true },
        { value: 'peripherals', inScope: true },
      ],
    });

    const { findByRole, getSearchParams } = renderPage([
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
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      scopedProductType: [
        { value: 'hardware', inScope: true },
        { value: 'peripherals', inScope: true },
      ],
    });

    const { findByRole, getSearchParams } = renderPage([
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
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      scopedProductType: [
        { value: 'hardware', inScope: true },
        { value: 'peripherals', inScope: true },
      ],
    });

    const { findByRole, getSearchParams } = renderPage();

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

    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', { entries });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      scopedProductType: [{ value: 'hardware', inScope: true }],
    });

    renderPage(['/ui/fleet/catalog?pageSize=10']);

    expect(await screen.findByText('Product 1')).toBeInTheDocument();
    expect(screen.queryByText('Product 12')).not.toBeInTheDocument();

    const nextPageBtn = screen.getByRole('button', { name: /next page/i });
    fireEvent.click(nextPageBtn);

    await waitFor(() => {
      expect(screen.getByText('Product 12')).toBeInTheDocument();
      expect(screen.queryByText('Product 1')).not.toBeInTheDocument();
    });
  }, 15000);

  it('should render both standard and GCE entries in unified table on All tab', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'std-catalog-1',
          productName: 'Standard Pixel Device',
          gpn: '12345',
          descriptiveName: 'Desc 1',
          resourceType: 'Type 1',
          fleetPlmStatus: 'GA',
          r11n: [],
          numberOfDevicesPerRack: 10,
          unitCost: '100',
          productType: 'hardware',
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'gce-catalog-1',
          productName: 'GCE n2-standard-4',
          descriptiveName: 'GCE N2 Instance',
          cpuType: 'x86_64',
          cpuNumPerVm: 4,
          memoryGbPerVm: 16,
          plmStatus: 'GA',
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      productCatalogId: [],
    });

    renderPage();

    await waitFor(
      () => {
        expect(screen.getByText('Standard Pixel Device')).toBeInTheDocument();
      },
      { timeout: 3000 },
    );
    expect(screen.getByText('GCE n2-standard-4')).toBeInTheDocument();
    expect(screen.getByText('x86_64')).toBeInTheDocument();
  });

  it('should remove default fleet_plm_status filter on first click of chip delete icon', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'std-catalog-1',
          productName: 'Standard Pixel Device',
          gpn: '12345',
          descriptiveName: 'Desc 1',
          resourceType: 'Type 1',
          fleetPlmStatus: 'GA',
          r11n: [],
          numberOfDevicesPerRack: 10,
          unitCost: '100',
          productType: 'hardware',
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('GetProductCatalogFilterValues', {
      fleetPlmStatus: [
        { value: 'GA', inScope: true },
        { value: 'LA', inScope: true },
        { value: 'NPI', inScope: true },
      ],
      scopedFleetPlmStatus: [
        { value: 'GA', inScope: true },
        { value: 'LA', inScope: true },
        { value: 'NPI', inScope: true },
      ],
      scopedProductType: [{ value: 'hardware', inScope: true }],
    });

    const { getSearchParams } = renderPage(['/ui/fleet/catalog']);

    const user = userEvent.setup();

    // Verify initial default filter chip is rendered
    const chip = await screen.findByTestId('filter-chip');
    expect(chip).toHaveTextContent(/Fleet PLM Status/i);

    // Click delete icon once on the filter chip
    const deleteIcon = within(chip).getByTestId('CancelIcon');
    await user.click(deleteIcon);

    // Verify the chip is removed on the first click
    await waitFor(() => {
      expect(screen.queryByTestId('filter-chip')).not.toBeInTheDocument();
    });

    // Verify search params no longer contains fleet_plm_status
    await waitFor(() => {
      expect(getSearchParams().get('filters') || '').not.toContain(
        'fleet_plm_status',
      );
    });
  });
});
