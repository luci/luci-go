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

import { render, screen } from '@testing-library/react';

import { SettingsProvider } from '@/fleet/context/providers';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import { ProductCatalogDetailsPage } from './product_catalogue_details_page';
import { useProductCatalogDetailsData } from './use_product_catalog_details_data';

jest.mock('./use_product_catalog_details_data');

const mockUseProductCatalogDetailsData =
  useProductCatalogDetailsData as jest.Mock;

const renderWithProviders = (ui: React.ReactNode) => {
  return render(
    <FakeContextProvider
      mountedPath="/catalog/:id"
      routerOptions={{
        initialEntries: ['/catalog/prod-12345'],
      }}
    >
      <SettingsProvider>{ui}</SettingsProvider>
    </FakeContextProvider>,
  );
};

describe('<ProductCatalogDetailsPage />', () => {
  it('renders loading by default', async () => {
    mockUseProductCatalogDetailsData.mockReturnValue({ isLoading: true });
    renderWithProviders(<ProductCatalogDetailsPage />);
    expect(screen.getByTestId('loading-spinner')).toBeVisible();
  });

  it('renders error state', async () => {
    mockUseProductCatalogDetailsData.mockReturnValue({
      isError: true,
      error: new Error('Fake network error'),
    });
    renderWithProviders(<ProductCatalogDetailsPage />);
    expect(
      screen.getByText(/Something went wrong:.*Fake network error/),
    ).toBeVisible();
  });

  it('renders not found state', async () => {
    mockUseProductCatalogDetailsData.mockReturnValue({
      isLoading: false,
      entry: undefined,
    });
    renderWithProviders(<ProductCatalogDetailsPage />);
    expect(screen.getByText('Entry not found!')).toBeVisible();
    expect(screen.getByText(/The product catalog entry/)).toBeVisible();
    const ids = screen.getAllByText('prod-12345');
    expect(ids.length).toBeGreaterThan(0);
    ids.forEach((el) => expect(el).toBeVisible());
  });

  it('renders details correctly', async () => {
    mockUseProductCatalogDetailsData.mockReturnValue({
      isLoading: false,
      entry: {
        productCatalogId: 'prod-12345',
        productName: 'Google Pixel 9 Pro',
        gpn: '123-4567-890',
        descriptiveName: 'Pixel 9 Pro 128GB Obsidian',
        resourceType: 'device',
        fleetPlmStatus: 'GA',
        r11n: ['r11n-us-central', 'r11n-us-east'],
        numberOfDevicesPerRack: 16,
        unitCost: '$999.00',
        productType: 'phone',
      },
    });
    renderWithProviders(<ProductCatalogDetailsPage />);

    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(
      'Product Catalog Entry:',
    );
    expect(screen.getByRole('textbox')).toHaveValue('prod-12345');

    // Verify fields
    expect(screen.getByText('Product Catalog ID')).toBeVisible();
    expect(screen.getByText('Product Name')).toBeVisible();
    expect(screen.getByText('Google Pixel 9 Pro')).toBeVisible();
    expect(screen.getByText('123-4567-890')).toBeVisible();
    expect(screen.getByText('Pixel 9 Pro 128GB Obsidian')).toBeVisible();
    expect(screen.getByText('device')).toBeVisible();
    expect(screen.getByText('GA')).toBeVisible();
    expect(screen.getByText('16')).toBeVisible();
    expect(screen.getByText('$999.00')).toBeVisible();
    expect(screen.getByText('phone')).toBeVisible();

    // Verify links
    const link1 = screen.getByText('r11n-us-central');
    expect(link1).toHaveAttribute(
      'href',
      'http://go/ngp-npi/r11n/r11n-us-central',
    );
    const link2 = screen.getByText('r11n-us-east');
    expect(link2).toHaveAttribute(
      'href',
      'http://go/ngp-npi/r11n/r11n-us-east',
    );
  });
});
