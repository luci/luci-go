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
  render,
  screen,
  fireEvent,
  waitForElementToBeRemoved,
} from '@testing-library/react';
import { DateTime } from 'luxon';

import { ProductCatalogEntry } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { OrderForm } from './order_form';

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

describe('<OrderForm />', () => {
  it('renders only the platform selector by default', () => {
    render(<OrderForm entry={mockEntry} />);

    expect(screen.getByText('Order Resources')).toBeVisible();
    expect(
      screen.getByLabelText(/Platform \(Fulfillment Channel\)/),
    ).toBeVisible();

    // Core fields should not be visible before platform is selected
    expect(screen.queryByLabelText(/Quantity/)).toBeNull();
    expect(screen.queryByLabelText(/Resource Group/)).toBeNull();
  });

  it('renders OS specific fields when OS platform is selected', async () => {
    render(<OrderForm entry={mockEntry} />);

    // Select OS Platform
    const platformSelect = screen.getByLabelText(
      /Platform \(Fulfillment Channel\)/,
    );
    fireEvent.mouseDown(platformSelect);
    const osOption = screen.getByText('Chrome OS');
    fireEvent.click(osOption);

    // Verify common fields are now visible
    expect(screen.getByLabelText(/Quantity/)).toBeVisible();
    expect(screen.getByLabelText(/Resource Group/)).toBeVisible();
    expect(screen.getByLabelText(/Criticality/)).toBeVisible();

    // Verify OS-specific fields are visible
    expect(screen.getByLabelText(/Swarming Server/)).toBeVisible();
    expect(screen.getByLabelText(/Swarming Pool/)).toBeVisible();
    expect(screen.getByLabelText(/NPI Approval/)).toBeVisible();
    expect(screen.getByLabelText(/NPI Type/)).toBeVisible();

    // Verify Android-specific fields are NOT visible
    expect(screen.queryByLabelText(/Host Group/)).toBeNull();
    expect(screen.queryByLabelText(/Mobile Harness/)).toBeNull();
  });

  it('renders Android specific fields when Android platform is selected', () => {
    render(<OrderForm entry={mockEntry} />);

    // Select Android Platform
    const platformSelect = screen.getByLabelText(
      /Platform \(Fulfillment Channel\)/,
    );
    fireEvent.mouseDown(platformSelect);
    const androidOption = screen.getByText('Android');
    fireEvent.click(androidOption);

    // Verify Android-specific fields are visible
    expect(screen.getByLabelText(/Host Group/)).toBeVisible();
    expect(screen.getByLabelText(/Mobile Harness/)).toBeVisible();

    // Verify OS-specific fields are NOT visible
    expect(screen.queryByLabelText(/NPI Approval/)).toBeNull();
    expect(screen.queryByLabelText(/NPI Type/)).toBeNull();
  });

  it('renders Mobile Harness sub-fields when Android is selected and Mobile Harness is Yes', () => {
    render(<OrderForm entry={mockEntry} />);

    // Select Android Platform
    const platformSelect = screen.getByLabelText(
      /Platform \(Fulfillment Channel\)/,
    );
    fireEvent.mouseDown(platformSelect);
    const androidOption = screen.getByText('Android');
    fireEvent.click(androidOption);

    // Mobile Harness fields shouldn't be visible by default
    expect(screen.queryByLabelText(/Mobile Harness Dimension/)).toBeNull();

    // Set Mobile Harness to Yes
    const mobileHarnessSelect = screen.getByLabelText(/Mobile Harness/);
    fireEvent.mouseDown(mobileHarnessSelect);
    const yesOption = screen.getByText('Yes');
    fireEvent.click(yesOption);

    // Verify Mobile Harness sub-fields are now visible
    expect(screen.getByLabelText(/Mobile Harness Dimension/)).toBeVisible();
    expect(screen.getByLabelText(/Mobile Harness WiFi/)).toBeVisible();
    expect(screen.getByLabelText(/Mobile Harness Owner/)).toBeVisible();
  });

  it('opens Buganizer URL in a new tab when the form is submitted', async () => {
    const openSpy = jest.spyOn(window, 'open').mockImplementation(() => null);
    render(<OrderForm entry={mockEntry} />);

    // Select Android Platform
    const platformSelect = screen.getByLabelText(
      /Platform \(Fulfillment Channel\)/,
    );
    fireEvent.mouseDown(platformSelect);
    const androidOption = screen.getByText('Android');
    fireEvent.click(androidOption);

    // Fill in required fields
    fireEvent.change(screen.getByLabelText(/Quantity/), {
      target: { value: '5' },
    });
    fireEvent.change(screen.getByLabelText(/Resource Group/), {
      target: { value: 'CrOS TryJob' },
    });
    fireEvent.change(screen.getByLabelText(/Business Justification/), {
      target: { value: 'We need these devices for testing custom kernels.' },
    });
    fireEvent.change(screen.getByLabelText(/Host Group/), {
      target: { value: 'atc:ate-main' },
    });
    fireEvent.change(screen.getByLabelText(/Estimated Launch Date/), {
      target: { value: '10/12/2026' },
    });

    // Select Mobile Harness No
    const mobileHarnessSelect = screen.getByLabelText(/Mobile Harness/);
    fireEvent.mouseDown(mobileHarnessSelect);
    const noOption = screen.getByText('No');
    fireEvent.click(noOption);

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
    expect(customFields).toContain('1399528:atc:ate-main'); // Host Group
    expect(customFields).toContain('1399763:No'); // Mobile Harness
    expect(customFields).toContain('1374342:Pixel 9 Pro 128GB Obsidian'); // Resource Name (Android)
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

  it('falls back to productName if descriptiveName is empty', async () => {
    const entryWithoutDesc = { ...mockEntry, descriptiveName: '' };
    render(<OrderForm entry={entryWithoutDesc} />);

    // Select OS Platform
    const platformSelect = screen.getByLabelText(
      /Platform \(Fulfillment Channel\)/,
    );
    fireEvent.mouseDown(platformSelect);
    const osOption = screen.getByText('Chrome OS');
    fireEvent.click(osOption);

    // Enter Business Justification
    fireEvent.change(screen.getByLabelText(/Business Justification/), {
      target: { value: 'Required validation value.' },
    });
    // Enter Resource Group
    fireEvent.change(screen.getByLabelText(/Resource Group/), {
      target: { value: 'CrOS TryJob' },
    });
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
    expect(customFields).toContain('1374341:Google Pixel 9 Pro'); // Resource Name (OS) fallback to productName
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
