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

import { fireEvent, render, screen, within } from '@testing-library/react';
import { useLocation } from 'react-router';

import { AvailableFlags } from '@/common/layouts/app_bar/available_flags/available_flags';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import { FleetConsoleMockAPI } from '@/fleet/testing_tools/mock_api';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  buildCatalogPhotoEditPrompt,
  COSMETIC_DROPBOX_GO_LINK,
  validatePhotoDimensions,
  validatePhotoUrlInput,
} from './catalog_edit_validation';
import { ProductCatalogDetailsPage } from './product_catalogue_details_page';

jest.mock('@/generic_libs/components/google_analytics', () => ({
  useGoogleAnalytics: () => ({ trackEvent: jest.fn() }),
}));

const renderDetailsPage = (catalogId: string) =>
  render(
    <FakeContextProvider
      mountedPath="/ui/fleet/catalog/:id"
      routerOptions={{ initialEntries: [`/ui/fleet/catalog/${catalogId}`] }}
    >
      <SettingsProvider>
        <ShortcutProvider>
          <ProductCatalogDetailsPage />
        </ShortcutProvider>
      </SettingsProvider>
    </FakeContextProvider>,
  );

describe('ProductCatalogDetailsPage', () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: {
        writeText: jest.fn().mockResolvedValue(undefined),
      },
    });
    FleetConsoleMockAPI.enableBrowserInterceptor();
    FleetConsoleMockAPI.resetFixtures();
    FleetConsoleMockAPI.setFixture(
      'GetResourceRequestsMultiselectFilterValues',
      {
        resourceGroups: ['cros-lab'],
      },
    );
  });

  afterEach(() => {
    jest.restoreAllMocks();
    FleetConsoleMockAPI.resetFixtures();
    localStorage.clear();
    sessionStorage.clear();
    window.history.replaceState({}, '', '/');
  });

  it('renders full-bleed hardware hero and carousel when photoDataUris is populated', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-000',
          productName: 'Android OmniLab Enclosure',
          gpn: '1055297-01',
          descriptiveName: 'Standard 16-slot testbed enclosure',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: ['r11n-atb'],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: [
            'data:image/webp;base64,mock-hero-photo-1',
            'data:image/webp;base64,mock-hero-photo-2',
          ],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-000');

    expect(
      await screen.findByTestId('catalog-details-hero'),
    ).toBeInTheDocument();
    const img = await screen.findByTestId('details-photo-img');
    expect(img).toHaveAttribute(
      'src',
      'data:image/webp;base64,mock-hero-photo-1',
    );
    expect(screen.getByText('1 / 2')).toBeInTheDocument();
    expect(screen.getAllByText('$26,645').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('Order Resources')).toBeInTheDocument();
    expect(
      screen.getByRole('columnheader', { name: 'Field' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('columnheader', { name: 'Value' }),
    ).toBeInTheDocument();
    expect(screen.queryByText('CPU Type')).not.toBeInTheDocument();
    expect(screen.queryByText('CPU Count Per VM')).not.toBeInTheDocument();
    expect(screen.queryByText('Memory (GB) Per VM')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Next photo' }));
    expect(screen.getByTestId('details-photo-img')).toHaveAttribute(
      'src',
      'data:image/webp;base64,mock-hero-photo-2',
    );
    expect(screen.getByText('2 / 2')).toBeInTheDocument();
  });

  it('renders intentional category blueprint fallback when photoDataUris is empty', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'PH-000',
          productName: 'Unphotographed Reference Device',
          gpn: '1066840-01',
          descriptiveName: 'Catalog SKU without reference photo',
          resourceType: 'PHONE',
          fleetPlmStatus: 'LA',
          r11n: [],
          numberOfDevicesPerRack: 24,
          unitCost: '850',
          productType: 'hardware',
          photoDataUris: [],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('PH-000');

    expect(
      await screen.findByTestId('details-photo-fallback'),
    ).toBeInTheDocument();
  });

  it('falls back to category blueprint when photo image triggers onError', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ERR-001',
          productName: 'Broken Photo Device',
          gpn: '1055297-03',
          descriptiveName: 'Device with broken photo URI',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: [],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: ['data:image/webp;base64,broken-uri'],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ERR-001');

    const img = await screen.findByTestId('details-photo-img');
    fireEvent.error(img);

    expect(
      await screen.findByTestId('details-photo-fallback'),
    ).toBeInTheDocument();
    expect(screen.queryByTestId('details-photo-img')).not.toBeInTheDocument();
  });

  it('navigates back to navigatedFromLink when clicking back arrow', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-000',
          productName: 'Android OmniLab Enclosure',
          gpn: '1055297-01',
          descriptiveName: 'Standard 16-slot testbed enclosure',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: [],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: [],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    const LocationDisplay = () => {
      const loc = useLocation();
      return (
        <div data-testid="location-display">{loc.pathname + loc.search}</div>
      );
    };

    render(
      <FakeContextProvider
        mountedPath="/ui/fleet/catalog/:id"
        routerOptions={{
          initialEntries: [
            {
              pathname: '/ui/fleet/catalog/ATB-000',
              state: { navigatedFromLink: '/ui/fleet/catalog?view=card' },
            },
          ],
        }}
        siblingRoutes={[
          {
            path: '/ui/fleet/catalog',
            element: <LocationDisplay />,
          },
        ]}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <ProductCatalogDetailsPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    const backButton = await screen.findByRole('button', { name: /go back/i });
    fireEvent.click(backButton);

    expect(await screen.findByTestId('location-display')).toHaveTextContent(
      '/ui/fleet/catalog?view=card',
    );
  });

  it('renders legacy details layout without photo hero when product-catalog-revamp feature flag is off', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-000',
          productName: 'Android OmniLab Enclosure',
          gpn: '1055297-01',
          descriptiveName: 'Standard 16-slot testbed enclosure',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: ['r11n-atb'],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: ['data:image/webp;base64,mock-hero-photo-1'],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-000?ff=product-catalog-revamp:off');

    expect(
      await screen.findByTestId('legacy-catalog-details-view'),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId('catalog-details-hero'),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole('columnheader', { name: 'Field' }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('columnheader', { name: 'Value' }),
    ).toBeInTheDocument();
  });

  it('toggles between revamped hero details and legacy details via the Feature Flags dialog', async () => {
    window.history.replaceState(
      {},
      '',
      '/ui/fleet/catalog/ATB-000?ff=product-catalog-revamp:on',
    );
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-000',
          productName: 'Android OmniLab Enclosure',
          gpn: '1055297-01',
          descriptiveName: 'Standard 16-slot testbed enclosure',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: ['r11n-atb'],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: ['data:image/webp;base64,mock-hero-photo-1'],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    render(
      <FakeContextProvider
        mountedPath="/ui/fleet/catalog/:id"
        routerOptions={{
          initialEntries: [
            '/ui/fleet/catalog/ATB-000?ff=product-catalog-revamp:on',
          ],
        }}
      >
        <SettingsProvider>
          <ShortcutProvider>
            <AvailableFlags />
            <ProductCatalogDetailsPage />
          </ShortcutProvider>
        </SettingsProvider>
      </FakeContextProvider>,
    );

    expect(
      await screen.findByTestId('catalog-details-hero'),
    ).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole('button', { name: /toggle feature flags/i }),
    );
    const flagSwitch = screen.getByTitle(
      'fleet-console:product-catalog-revamp switch',
    );
    const switchInput =
      within(flagSwitch).queryByRole('checkbox') ?? flagSwitch;
    fireEvent.click(switchInput);

    expect(
      await screen.findByTestId('legacy-catalog-details-view'),
    ).toBeInTheDocument();

    fireEvent.click(switchInput);
    expect(
      await screen.findByTestId('catalog-details-hero'),
    ).toBeInTheDocument();
  });

  it('renders loading spinner while catalog entries are loading', () => {
    FleetConsoleMockAPI.setFixture(
      'ListProductCatalogEntries',
      () => new Promise(() => {}),
    );
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-000');

    expect(screen.getByTestId('loading-spinner')).toBeInTheDocument();
  });

  it('renders error alert when ListProductCatalogEntries fails', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', () => {
      throw new Error('Catalog service unavailable');
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-000');

    expect(
      await screen.findByText(/Something went wrong/i),
    ).toBeInTheDocument();
  });

  it('renders Entry not found alert when catalog ID is missing', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('MISSING-999');

    expect(await screen.findByText('Entry not found!')).toBeInTheDocument();
  });

  it('renders non-numeric unitCost as raw text without a leading dollar sign', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-001',
          productName: 'Pending Price Enclosure',
          gpn: '1055297-02',
          descriptiveName: 'Enclosure with non-numeric unitCost',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'NPI',
          r11n: [],
          numberOfDevicesPerRack: 4,
          unitCost: 'TBD',
          productType: 'android-testbed',
          photoDataUris: [],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-001');

    expect(
      await screen.findByTestId('catalog-details-hero'),
    ).toBeInTheDocument();
    expect(screen.getAllByText('TBD').length).toBeGreaterThanOrEqual(1);
    expect(screen.queryByText('$TBD')).not.toBeInTheDocument();
  });

  it('renders GCE entry specifications and hero metrics while omitting hardware-only fields', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'GCE-001',
          productName: 'n2-standard-8',
          descriptiveName: 'Standard 8-vCPU GCE instance',
          fleetPlmStatus: 'GA',
          cpuType: 'Intel Cascade Lake',
          cpuNumPerVm: 8,
          memoryGbPerVm: 32,
        },
      ],
    });

    renderDetailsPage('GCE-001');

    expect(
      await screen.findByTestId('catalog-details-hero'),
    ).toBeInTheDocument();
    expect(screen.getByText('GCE Virtual Machine')).toBeInTheDocument();
    expect(screen.getByText('CPU Architecture')).toBeInTheDocument();
    expect(
      screen.getAllByText('Intel Cascade Lake').length,
    ).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('VM Compute Profile')).toBeInTheDocument();
    expect(screen.getByText('8 vCPU • 32 GB')).toBeInTheDocument();
    expect(screen.getByText('Fulfillment Model')).toBeInTheDocument();
    expect(screen.getByText('Cloud Virtual Instance')).toBeInTheDocument();
    expect(screen.queryByText('R11N Build Guides')).not.toBeInTheDocument();
    expect(screen.getByText('CPU Type')).toBeInTheDocument();
    expect(screen.getByText('CPU Count Per VM')).toBeInTheDocument();
    expect(screen.getByText('Memory (GB) Per VM')).toBeInTheDocument();
    expect(screen.queryByText('GPN')).not.toBeInTheDocument();
    expect(screen.queryByText('Resource Type')).not.toBeInTheDocument();
    expect(
      screen.queryByText('Number of Devices Per Rack'),
    ).not.toBeInTheDocument();
  });

  it('stages photo URL edits, displays the always-visible unsaved toast, and generates Jetski & Captain Fin prompts', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-000',
          productName: 'Android OmniLab Enclosure',
          gpn: '1055297-01',
          descriptiveName: 'Standard 16-slot testbed enclosure',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: ['r11n-atb'],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: ['data:image/webp;base64,mock-hero-photo-1'],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-000');

    const editBtn = await screen.findByTestId('toggle-photo-editor-btn');
    fireEvent.click(editBtn);

    expect(
      screen.getByTestId('catalog-photo-editor-panel'),
    ).toBeInTheDocument();
    expect(screen.getByTestId('bulk-photo-dropbox-note')).toHaveTextContent(
      COSMETIC_DROPBOX_GO_LINK,
    );

    // Reject invalid photo URL
    const urlInput = screen.getByTestId('photo-url-input');
    fireEvent.change(urlInput, { target: { value: 'not-a-valid-url' } });
    fireEvent.click(screen.getByTestId('add-photo-url-btn'));
    expect(
      screen.getByText(/Use an https:\/\/ image URL/i),
    ).toBeInTheDocument();

    // Stage a valid http://screen/ URL
    fireEvent.change(urlInput, {
      target: { value: 'http://screen/9kLmN2pQrStUvWx' },
    });
    fireEvent.click(screen.getByTestId('add-photo-url-btn'));

    // Promote new photo to primary
    fireEvent.click(screen.getByTestId('set-primary-photo-1'));
    expect(screen.getByTestId('details-photo-img')).toHaveAttribute(
      'src',
      'http://screen/9kLmN2pQrStUvWx',
    );

    // Copy prompt directly via "Copy prompt to save" inside the photo modal (no second modal)
    fireEvent.click(screen.getByTestId('save-photos-by-prompt-btn'));
    expect(
      screen.queryByTestId('catalog-edit-prompt-dialog'),
    ).not.toBeInTheDocument();
    expect(
      (await screen.findAllByTestId('handoff-guidance'))[0],
    ).toHaveTextContent(/Copied prompt for ATB-000/i);

    const openJetskiBtns = screen.getAllByTestId('open-in-jetski-btn');
    expect(openJetskiBtns[0].getAttribute('href')).toContain(
      'http://goto.google.com/jetski-web?q=',
    );
    expect(
      screen.getAllByTestId('open-captain-fin-btn')[0].getAttribute('href'),
    ).toBe('http://goto.google.com/captain-fin');
    expect(
      screen.getAllByTestId('generated-catalog-prompt')[0],
    ).toHaveTextContent('http://screen/9kLmN2pQrStUvWx (primary hero photo)');

    // Close modal via "Keep editing" and verify the bottom unsaved changes toast renders cleanly
    fireEvent.click(screen.getByTestId('keep-editing-photos-btn'));
    const toast = await screen.findByTestId('unsaved-changes-toast');
    expect(toast).toHaveTextContent(/Copied prompt/i);
  });

  it('allows deleting a photo without triggering ghost validation errors', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-000',
          productName: 'Android OmniLab Enclosure',
          gpn: '1055297-01',
          descriptiveName: 'Standard 16-slot testbed enclosure',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: ['r11n-atb'],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: [
            'data:image/webp;base64,mock-hero-photo-1',
            'data:image/webp;base64,mock-hero-photo-2',
          ],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-000');

    const editBtn = await screen.findByTestId('toggle-photo-editor-btn');
    fireEvent.click(editBtn);

    // Trigger an input error on the empty draft URL field first, then delete an existing photo
    fireEvent.click(screen.getByTestId('add-photo-url-btn'));
    fireEvent.click(screen.getByTestId('remove-photo-1'));
    fireEvent.click(screen.getByTestId('keep-editing-photos-btn'));

    const toast = await screen.findByTestId('unsaved-changes-toast');
    expect(toast).toHaveTextContent('1 unsaved change');
    expect(toast).not.toHaveTextContent('validation error');
    expect(screen.getByTestId('review-generate-prompt-btn')).not.toBeDisabled();
  });

  it('validates photo URLs, dimensions, and preserves original existing_photo index on reorder', () => {
    expect(validatePhotoUrlInput('', [])).toMatch(/Enter an image URL/i);
    expect(validatePhotoUrlInput('ftp://bad', [])).toMatch(
      /Use an https:\/\/ image URL/i,
    );
    expect(
      validatePhotoUrlInput('http://screen/abc123', ['http://screen/abc123']),
    ).toMatch(/already staged/i);
    expect(validatePhotoUrlInput('http://screen/abc123', [])).toBeUndefined();

    expect(validatePhotoDimensions(180, 100)).toMatch(/Minimum size/i);
    expect(validatePhotoDimensions(1000, 200)).toMatch(/Avoid 1U rack-strip/i);
    expect(validatePhotoDimensions(700, 368)).toBeUndefined();

    const prompt = buildCatalogPhotoEditPrompt(
      {
        productCatalogId: 'ATB-000',
        productName: 'Android OmniLab Enclosure',
        gpn: '1055297-01',
        descriptiveName: 'Standard 16-slot testbed enclosure',
        resourceType: 'TESTBED',
        fleetPlmStatus: 'GA',
        r11n: [],
        numberOfDevicesPerRack: 4,
        unitCost: '26645',
        productType: 'android-testbed',
        photoDataUris: [
          'data:image/webp;base64,photo-1',
          'data:image/webp;base64,photo-2',
        ],
      },
      ['data:image/webp;base64,photo-2', 'data:image/webp;base64,photo-1'],
    );
    expect(prompt).toContain(
      '1. existing_photo_2.webp (retained catalog image) (primary hero photo)',
    );
    expect(prompt).toContain(
      '2. existing_photo_1.webp (retained catalog image)',
    );
  });

  it('validates inline spec edits, supports click-to-edit, and combines diffs in the prompt', async () => {
    FleetConsoleMockAPI.setFixture('ListProductCatalogEntries', {
      entries: [
        {
          productCatalogId: 'ATB-000',
          productName: 'Android OmniLab Enclosure',
          gpn: '1055297-01',
          descriptiveName: 'Standard 16-slot testbed enclosure',
          resourceType: 'TESTBED',
          fleetPlmStatus: 'GA',
          r11n: ['r11n-atb'],
          numberOfDevicesPerRack: 4,
          unitCost: '26645',
          productType: 'android-testbed',
          photoDataUris: ['data:image/webp;base64,mock-hero-photo-1'],
        },
      ],
    });
    FleetConsoleMockAPI.setFixture('ListGceProductCatalogEntries', {
      entries: [],
    });

    renderDetailsPage('ATB-000');

    // Click directly on an editable specification cell in read mode to enter edit mode
    const gpnReadCell = await screen.findByTestId('spec-cell-gpn');
    fireEvent.click(gpnReadCell);

    const gpnInput = await screen.findByTestId('spec-input-gpn');
    const unitCostInput = screen.getByTestId('spec-input-unitCost');

    // Enter an invalid GPN and verify inline error + disabled prompt button in toast
    fireEvent.change(gpnInput, { target: { value: 'invalid-gpn!' } });
    expect(
      screen.getAllByText(/Enter a valid part number/i).length,
    ).toBeGreaterThanOrEqual(1);
    const toastWithError = await screen.findByTestId('unsaved-changes-toast');
    expect(toastWithError).toHaveTextContent('1 validation error');
    expect(toastWithError).toHaveTextContent(/Enter a valid part number/i);
    expect(screen.getByTestId('review-generate-prompt-btn')).toBeDisabled();

    // Fix GPN, update Unit Cost, and select a new Fleet PLM Status from the MUI Select dropdown
    fireEvent.change(gpnInput, { target: { value: '1055297-02' } });
    fireEvent.change(unitCostInput, { target: { value: '28500' } });

    const plmSelect = screen.getByRole('combobox', {
      name: 'Edit Fleet PLM Status',
    });
    fireEvent.mouseDown(plmSelect);
    const laOption = await screen.findByRole('option', { name: 'LA' });
    expect(screen.getByRole('option', { name: 'GA' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'NPI' })).toBeInTheDocument();
    fireEvent.click(laOption);

    expect(screen.getByTestId('unsaved-changes-toast')).toHaveTextContent(
      '3 unsaved changes',
    );
    expect(screen.getByTestId('review-generate-prompt-btn')).not.toBeDisabled();

    fireEvent.click(screen.getByTestId('review-generate-prompt-btn'));
    const promptPre = await screen.findByTestId('generated-catalog-prompt');
    expect(promptPre).toHaveTextContent('GPN: 1055297-01 -> 1055297-02');
    expect(promptPre).toHaveTextContent('Fleet PLM Status: GA -> LA');
    expect(promptPre).toHaveTextContent('Unit Cost: 26645 -> 28500');
    expect(screen.getByTestId('handoff-guidance')).toHaveTextContent(
      /Copied prompt for ATB-000/i,
    );

    // Verify Discard resets edits and hides the toast
    fireEvent.click(screen.getByTestId('discard-catalog-edits-btn'));
    expect(
      screen.queryByTestId('unsaved-changes-toast'),
    ).not.toBeInTheDocument();
  });
});
