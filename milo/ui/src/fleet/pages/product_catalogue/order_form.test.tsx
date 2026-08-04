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
import {
  render,
  screen,
  fireEvent,
  waitFor,
  waitForElementToBeRemoved,
} from '@testing-library/react';
import { DateTime } from 'luxon';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { OrderForm } from './order_form';

jest.mock('@/fleet/hooks/prpc_clients');

const mockEntry: ProductCatalogEntry = {
  productCatalogId: 'prod-12345',
  productName: 'Google Pixel 9 Pro',
  gpn: '123-4567-890',
  descriptiveName: 'Pixel 9 Pro 128GB Obsidian',
  resourceType: 'device',
  fleetPlmStatus: 'GA',
  r11n: [],
  numberOfDevicesPerRack: 16,
  unitCost: '$999.00',
  productType: 'phone',
};

const renderOrderForm = (
  entry: ProductCatalogEntry = mockEntry,
  resourceGroups: string[] = ['CrOS TryJob', 'Group A', 'Group B'],
) => {
  const mockUseFleetConsoleClient = useFleetConsoleClient as jest.Mock;
  mockUseFleetConsoleClient.mockReturnValue({
    GetResourceRequestsMultiselectFilterValues: {
      query: () => ({
        queryKey: ['GetResourceRequestsMultiselectFilterValues'],
        queryFn: async () => ({
          resourceGroups,
        }),
      }),
    },
  });

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <OrderForm entry={entry} />
    </QueryClientProvider>,
  );
};

describe('<OrderForm />', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders only the platform selector by default', () => {
    renderOrderForm();

    expect(screen.getByText('Order Resources')).toBeVisible();
    expect(
      screen.getByRole('combobox', {
        name: /Platform \(Fulfillment Channel\)/,
      }),
    ).toBeVisible();

    // Core fields should not be visible before platform is selected
    expect(screen.queryByLabelText(/Quantity/)).toBeNull();
    expect(
      screen.queryByRole('combobox', { name: /Resource Group/ }),
    ).toBeNull();
  });

  it('renders common fields when OS platform is selected', async () => {
    renderOrderForm();

    // Select OS Platform
    const platformSelect = screen.getByRole('combobox', {
      name: /Platform \(Fulfillment Channel\)/,
    });
    fireEvent.mouseDown(platformSelect);
    const osOption = screen.getByText('Chrome OS');
    fireEvent.click(osOption);

    // Verify common fields are now visible
    expect(screen.getByLabelText(/Quantity/)).toBeVisible();
    expect(
      await screen.findByRole('combobox', { name: /Resource Group/ }),
    ).toBeVisible();
    expect(screen.getByRole('combobox', { name: /Criticality/ })).toBeVisible();

    // Verify Android-specific fields are NOT visible
    expect(screen.queryByLabelText(/Mobile Harness/)).toBeNull();
  });

  it('populates Resource Group dropdown from backend data', async () => {
    renderOrderForm(mockEntry, ['Alpha Group', 'Beta Group']);

    // Select OS Platform
    const platformSelect = screen.getByRole('combobox', {
      name: /Platform \(Fulfillment Channel\)/,
    });
    fireEvent.mouseDown(platformSelect);
    fireEvent.click(screen.getByText('Chrome OS'));

    // Open Resource Group dropdown
    const resourceGroupSelect = await screen.findByRole('combobox', {
      name: /Resource Group/,
    });
    await waitFor(() => {
      expect(resourceGroupSelect).not.toHaveAttribute('aria-disabled', 'true');
    });
    fireEvent.mouseDown(resourceGroupSelect);

    expect(await screen.findByText('Alpha Group')).toBeInTheDocument();
    expect(await screen.findByText('Beta Group')).toBeInTheDocument();
  });

  it('renders Mobile Harness sub-fields automatically when Android platform is selected and entry productType is android-testbed', () => {
    const testbedEntry = { ...mockEntry, productType: 'android-testbed' };
    renderOrderForm(testbedEntry);

    // Select Android Platform
    const platformSelect = screen.getByRole('combobox', {
      name: /Platform \(Fulfillment Channel\)/,
    });
    fireEvent.mouseDown(platformSelect);
    const androidOption = screen.getByText('Android');
    fireEvent.click(androidOption);

    // Verify Mobile Harness sub-fields are visible by default
    expect(screen.getByLabelText(/Mobile Harness Dimension/)).toBeVisible();
    expect(
      screen.getByRole('combobox', { name: /Mobile Harness WiFi/ }),
    ).toBeVisible();
    expect(screen.getByLabelText(/Mobile Harness Owner/)).toBeVisible();
    expect(screen.queryByLabelText(/^Mobile Harness$/)).toBeNull();
  });

  it('opens Buganizer URL in a new tab when the form is submitted', async () => {
    const openSpy = jest.spyOn(window, 'open').mockImplementation(() => null);
    renderOrderForm();

    // Select Android Platform
    const platformSelect = screen.getByRole('combobox', {
      name: /Platform \(Fulfillment Channel\)/,
    });
    fireEvent.mouseDown(platformSelect);
    const androidOption = screen.getByText('Android');
    fireEvent.click(androidOption);

    // Fill in required fields
    fireEvent.change(screen.getByLabelText(/Quantity/), {
      target: { value: '5' },
    });

    // Select Resource Group
    const resourceGroupSelect = await screen.findByRole('combobox', {
      name: /Resource Group/,
    });
    await waitFor(() => {
      expect(resourceGroupSelect).not.toHaveAttribute('aria-disabled', 'true');
    });
    fireEvent.mouseDown(resourceGroupSelect);
    const groupOption = await screen.findByText('CrOS TryJob');
    fireEvent.click(groupOption);

    fireEvent.change(screen.getByLabelText(/Business Justification/), {
      target: { value: 'We need these devices for testing custom kernels.' },
    });
    fireEvent.change(screen.getByLabelText(/Estimated Launch Date/), {
      target: { value: '10/12/2026' },
    });

    // Submit form
    const submitButton = screen.getByRole('button', { name: 'Submit Order' });
    const form = submitButton.closest('form');
    expect(form).not.toBeNull();
    if (form) {
      fireEvent.submit(form);
    }

    expect(openSpy).toHaveBeenCalledTimes(1);
    const openedUrlStr = openSpy.mock.calls[0][0] as string;
    const url = new URL(openedUrlStr);

    expect(url.origin).toBe('https://b.corp.google.com');
    expect(url.pathname).toBe('/issues/new');
    expect(url.searchParams.get('component')).toBe('1642317');
    expect(url.searchParams.get('template')).toBe('2040931');
    expect(url.searchParams.get('title')).toBe(
      '[Resource Request] 5 x Google Pixel 9 Pro for CrOS TryJob',
    );
    expect(url.searchParams.get('description')).toBe(
      'Business Justification: We need these devices for testing custom kernels.\n\n' +
        'Product Catalog Name: Google Pixel 9 Pro\n' +
        'Catalog ID: prod-12345',
    );

    // Verify custom fields
    const customFields = url.searchParams.getAll('customFields');
    expect(customFields).toContain('1320241:Android'); // Fulfillment Channel
    expect(customFields).toContain('1398911:prod-12345'); // Catalog ID
    expect(customFields).toContain('1399654:5'); // Quantity
    expect(customFields).toContain('1473369:CrOS TryJob'); // Resource Group
    expect(customFields).toContain('1399763:No'); // Mobile Harness
    expect(customFields).toContain('1374342:Google Pixel 9 Pro'); // Resource Name (Android)
    const expectedDate = DateTime.fromFormat(
      '10/12/2026',
      'MM/dd/yyyy',
    ).toFormat('M/d/yyyy');
    expect(customFields).toContain(`1398937:${expectedDate}`); // Estimated Launch Date

    // Verify modal is open
    expect(screen.getByText('Order Submitted')).toBeInTheDocument();
    expect(
      screen.getByText(/A pre-populated Buganizer issue has been generated/),
    ).toBeInTheDocument();

    // Verify manual link
    const manualLink = screen.getByText('click here to open it manually');
    expect(manualLink).toBeInTheDocument();
    expect(manualLink.getAttribute('href')).toBe(openedUrlStr);

    // Verify RRI link
    const rriLink = screen.getByText('Resource Request Insights (RRI)');
    expect(rriLink).toBeInTheDocument();
    expect(rriLink.getAttribute('href')).toBe('/ui/fleet/labs/requests');

    // Click close button
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    // Verify modal is closed
    await waitForElementToBeRemoved(() =>
      screen.queryByText('Order Submitted'),
    );

    openSpy.mockRestore();
  });

  it('uses productName for Resource Name', async () => {
    const entryWithoutDesc = { ...mockEntry, descriptiveName: '' };
    renderOrderForm(entryWithoutDesc);

    // Select OS Platform
    const platformSelect = screen.getByRole('combobox', {
      name: /Platform \(Fulfillment Channel\)/,
    });
    fireEvent.mouseDown(platformSelect);
    const osOption = screen.getByText('Chrome OS');
    fireEvent.click(osOption);

    // Enter Business Justification
    fireEvent.change(screen.getByLabelText(/Business Justification/), {
      target: { value: 'Required validation value.' },
    });

    // Select Resource Group
    const resourceGroupSelect = await screen.findByRole('combobox', {
      name: /Resource Group/,
    });
    await waitFor(() => {
      expect(resourceGroupSelect).not.toHaveAttribute('aria-disabled', 'true');
    });
    fireEvent.mouseDown(resourceGroupSelect);
    const groupOption = await screen.findByText('CrOS TryJob');
    fireEvent.click(groupOption);

    // Enter Launch Date
    fireEvent.change(screen.getByLabelText(/Estimated Launch Date/), {
      target: { value: '10/12/2026' },
    });

    const openSpy = jest.spyOn(window, 'open').mockImplementation(() => null);

    const submitButton = screen.getByRole('button', { name: 'Submit Order' });
    const form = submitButton.closest('form');
    expect(form).not.toBeNull();
    if (form) {
      fireEvent.submit(form);
    }

    expect(openSpy).toHaveBeenCalledTimes(1);
    const openedUrlStr = openSpy.mock.calls[0][0] as string;
    const url = new URL(openedUrlStr);
    const customFields = url.searchParams.getAll('customFields');
    expect(customFields).toContain('1374341:Google Pixel 9 Pro'); // Resource Name (OS) uses productName
    const expectedDate = DateTime.fromFormat(
      '10/12/2026',
      'MM/dd/yyyy',
    ).toFormat('M/d/yyyy');
    expect(customFields).toContain(`1398937:${expectedDate}`); // Estimated Launch Date

    // Verify modal is open
    expect(screen.getByText('Order Submitted')).toBeInTheDocument();
    expect(
      screen.getByText(/A pre-populated Buganizer issue has been generated/),
    ).toBeInTheDocument();

    // Verify manual link
    const manualLink = screen.getByText('click here to open it manually');
    expect(manualLink).toBeInTheDocument();
    expect(manualLink.getAttribute('href')).toBe(openedUrlStr);

    // Verify RRI link
    const rriLink = screen.getByText('Resource Request Insights (RRI)');
    expect(rriLink).toBeInTheDocument();
    expect(rriLink.getAttribute('href')).toBe('/ui/fleet/labs/requests');

    // Click close button
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));

    // Verify modal is closed
    await waitForElementToBeRemoved(() =>
      screen.queryByText('Order Submitted'),
    );

    openSpy.mockRestore();
  });
});
