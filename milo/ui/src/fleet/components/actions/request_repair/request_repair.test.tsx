// Copyright 2025 The LUCI Authors.
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

import { fireEvent, render, screen } from '@testing-library/react';

import {
  AndroidDevice,
  Platform,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { HealthCategory } from '@/proto/go.chromium.org/infra/fleetconsole/internal/infra/ext/omnilab/omnilab-device.pb';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';

import { DutToRepair } from '../shared/types';

import { RequestRepair } from './request_repair';
import {
  BrowserDeviceToRepair,
  BrowserRepairConfig,
} from './request_repair_browser_config';
import { ChromeOSRepairConfig } from './request_repair_os_config';

describe('<RequestRepair />', () => {
  let windowOpenSpy: jest.SpyInstance;

  beforeEach(() => {
    windowOpenSpy = jest.spyOn(window, 'open').mockImplementation(jest.fn());
  });

  afterEach(() => {
    windowOpenSpy.mockRestore();
  });

  it('should render a button that opens a new tab with the correct URL', () => {
    const selectedDuts: DutToRepair[] = [
      {
        name: 'dut1',
        dutId: 'dut1',
        state: 'needs_manual_repair',
        board: 'board1',
        model: 'model1',
        pool: 'pool1',
      },
      {
        name: 'dut2',
        dutId: 'dut2',
        state: 'needs_manual_repair',
        board: 'board2',
        model: 'model2',
        pool: 'pool2',
      },
      {
        name: 'dut3',
        dutId: 'dut3',
        state: 'ready',
        board: 'board3',
        model: 'model3',
        pool: 'pool3',
      },
      {
        name: 'dut4',
        dutId: 'dut4',
        state: 'repair_failed',
        board: 'board4',
        model: 'model4',
        pool: 'pool4',
      },
    ];

    render(
      <FakeAuthStateProvider>
        <RequestRepair
          selectedItems={selectedDuts}
          platform={Platform.CHROMEOS}
        />
      </FakeAuthStateProvider>,
    );
    const button = screen.getByTestId('file-repair-bug-button');
    expect(button).toBeInTheDocument();

    fireEvent.click(button);

    expect(windowOpenSpy).toHaveBeenCalledTimes(1);
    const expectedDescription =
      ChromeOSRepairConfig.generateDescription(selectedDuts);

    const openedUrl = new URL(windowOpenSpy.mock.calls[0][0]);
    expect(openedUrl.origin).toBe('http://b');
    expect(openedUrl.pathname).toBe('/issues/new');
    expect(openedUrl.searchParams.get('format')).toBe('MARKDOWN');
    expect(openedUrl.searchParams.get('component')).toBe('575445');
    expect(openedUrl.searchParams.get('template')).toBe('1509031');
    expect(openedUrl.searchParams.get('title')).toBe(
      `[Location Unknown][Repair][board1.model1] Pool: [pool1] [dut1] and 3 more`,
    );
    expect(decodeURIComponent(openedUrl.searchParams.get('description')!)).toBe(
      decodeURIComponent(expectedDescription),
    );
    expect(windowOpenSpy).toHaveBeenCalledWith(
      expect.stringContaining(''),
      '_blank',
    );
  });

  it.each([
    {
      case: 'no DUTs are selected',
      duts: [] as DutToRepair[],
    },
  ])('should not render the button if $case', ({ duts }) => {
    render(
      <FakeAuthStateProvider>
        <RequestRepair selectedItems={duts} platform={Platform.CHROMEOS} />
      </FakeAuthStateProvider>,
    );
    const button = screen.queryByTestId('file-repair-bug-button');
    expect(button).not.toBeInTheDocument();
  });

  it('should render a button that opens a new tab with the correct URL for single repair (Browser)', () => {
    const selectedDevices: BrowserDeviceToRepair[] = [
      { id: 'machine1', hostname: 'host1', pool: 'pool1' },
    ];

    render(
      <FakeAuthStateProvider>
        <RequestRepair
          selectedItems={selectedDevices}
          platform={Platform.CHROMIUM}
        />
      </FakeAuthStateProvider>,
    );
    const button = screen.getByTestId('file-repair-bug-button');
    expect(button).toBeInTheDocument();

    fireEvent.click(button);

    expect(windowOpenSpy).toHaveBeenCalledTimes(1);
    const expectedDescription = BrowserRepairConfig.generateDescription([
      { id: 'machine1', hostname: 'host1', pool: 'pool1' },
    ]);

    const openedUrl = new URL(windowOpenSpy.mock.calls[0][0]);
    expect(openedUrl.origin).toBe('http://b');
    expect(openedUrl.pathname).toBe('/issues/new');
    expect(openedUrl.searchParams.get('format')).toBe('MARKDOWN');
    expect(openedUrl.searchParams.get('component')).toBe('1735976');
    expect(openedUrl.searchParams.get('template')).toBe('2107381');
    expect(openedUrl.searchParams.get('title')).toBe(
      '[Unknown Zone][Browser][Repair][host1]',
    );
    expect(decodeURIComponent(openedUrl.searchParams.get('description')!)).toBe(
      decodeURIComponent(expectedDescription),
    );
  });

  it('should render correct title for exactly 2 devices (Browser)', () => {
    const selectedDevices: BrowserDeviceToRepair[] = [
      { id: 'machine1', hostname: 'host1', pool: 'pool1' },
      { id: 'machine2', hostname: 'host2', pool: 'pool2' },
    ];

    render(
      <FakeAuthStateProvider>
        <RequestRepair
          selectedItems={selectedDevices}
          platform={Platform.CHROMIUM}
        />
      </FakeAuthStateProvider>,
    );
    const button = screen.getByTestId('file-repair-bug-button');
    fireEvent.click(button);

    const openedUrl = new URL(windowOpenSpy.mock.calls[0][0]);
    expect(openedUrl.searchParams.get('title')).toBe(
      '[Unknown Zone][Browser][Repair][host1, host2]',
    );
  });

  it('should render a button that opens a new tab with the correct URL for bulk repair (Browser)', () => {
    const selectedDevices: BrowserDeviceToRepair[] = [
      { id: 'machine1', hostname: 'host1', pool: 'pool1' },
      { id: 'machine2', hostname: 'host2', pool: 'pool2' },
      { id: 'machine3', hostname: 'host3', pool: 'pool3' },
    ];

    render(
      <FakeAuthStateProvider>
        <RequestRepair
          selectedItems={selectedDevices}
          platform={Platform.CHROMIUM}
        />
      </FakeAuthStateProvider>,
    );
    const button = screen.getByTestId('file-repair-bug-button');
    expect(button).toBeInTheDocument();

    fireEvent.click(button);

    expect(windowOpenSpy).toHaveBeenCalledTimes(1);
    const expectedDescription =
      BrowserRepairConfig.generateDescription(selectedDevices);

    const openedUrl = new URL(windowOpenSpy.mock.calls[0][0]);
    expect(openedUrl.origin).toBe('http://b');
    expect(openedUrl.pathname).toBe('/issues/new');
    expect(openedUrl.searchParams.get('component')).toBe('1735976');
    expect(openedUrl.searchParams.get('template')).toBe('2161122');
    expect(openedUrl.searchParams.get('title')).toBe(
      '[Unknown Zone][Browser] [Repair] [3] - [Multiple Devices]',
    );
    expect(decodeURIComponent(openedUrl.searchParams.get('description')!)).toBe(
      decodeURIComponent(expectedDescription),
    );
  });

  describe('Pixel restoration', () => {
    const createPixelDevice = (
      id: string,
      labName?: string,
    ): AndroidDevice => ({
      id,
      runTarget: 'oriole',
      realm: 'pixel-realm',
      healthCategory: HealthCategory.HEALTH_CATEGORY_IN_SERVICE,
      fcOfflineSince: undefined,
      omnilabSpec:
        labName !== undefined
          ? {
              labels: {
                lab_name: { values: [labName] },
                hostname: { values: [`${id}.mock.host`] },
                dut_state: { values: ['ready'] },
              },
            }
          : { labels: {} },
    });

    it('should not render the button if no Pixel devices are selected', () => {
      render(
        <FakeAuthStateProvider>
          <RequestRepair selectedItems={[]} platform={Platform.PIXEL} />
        </FakeAuthStateProvider>,
      );
      expect(
        screen.queryByTestId('file-restoration-bug-button'),
      ).not.toBeInTheDocument();
    });

    it('should render a button that opens a new tab with the prepopulated template for single device', () => {
      const dev = createPixelDevice('pixel-dev-1', 'pixel-lab-mtv');

      render(
        <FakeAuthStateProvider>
          <RequestRepair selectedItems={[dev]} platform={Platform.PIXEL} />
        </FakeAuthStateProvider>,
      );

      const button = screen.getByTestId('file-restoration-bug-button');
      expect(button).toBeInTheDocument();
      expect(button).toBeEnabled();
      expect(button).toHaveTextContent('Request Restoration');

      fireEvent.click(button);

      expect(windowOpenSpy).toHaveBeenCalledTimes(1);
      const openedUrl = new URL(windowOpenSpy.mock.calls[0][0]);
      expect(openedUrl.origin).toBe('https://b.corp.google.com');
      expect(openedUrl.pathname).toBe('/issues/new');
      expect(openedUrl.searchParams.get('component')).toBe('2174751');
      expect(openedUrl.searchParams.get('template')).toBe('2403455');
      expect(openedUrl.searchParams.get('format')).toBe('MARKDOWN');
      expect(openedUrl.searchParams.get('title')).toBe(
        '[pixel-lab-mtv][Restoration][pixel-dev-1]',
      );

      const desc = decodeURIComponent(
        openedUrl.searchParams.get('description')!,
      );
      expect(desc.startsWith('Lab: pixel\\-lab\\-mtv')).toBe(true);
      expect(desc).toContain('pixel-dev-1');
    });

    it('should render an enabled button for multiple devices from the same lab', () => {
      const dev1 = createPixelDevice('pixel-dev-1', 'pixel-lab-mtv');
      const dev2 = createPixelDevice('pixel-dev-2', 'pixel-lab-mtv');

      render(
        <FakeAuthStateProvider>
          <RequestRepair
            selectedItems={[dev1, dev2]}
            platform={Platform.PIXEL}
          />
        </FakeAuthStateProvider>,
      );

      const button = screen.getByTestId('file-restoration-bug-button');
      expect(button).toBeInTheDocument();
      expect(button).toBeEnabled();

      fireEvent.click(button);

      expect(windowOpenSpy).toHaveBeenCalledTimes(1);
      const openedUrl = new URL(windowOpenSpy.mock.calls[0][0]);
      expect(openedUrl.origin).toBe('https://b.corp.google.com');
      expect(openedUrl.searchParams.get('component')).toBe('2174751');
      expect(openedUrl.searchParams.get('template')).toBe('2403455');
      expect(openedUrl.searchParams.get('title')).toBe(
        '[pixel-lab-mtv][Restoration][pixel-dev-1, pixel-dev-2]',
      );

      const desc = decodeURIComponent(
        openedUrl.searchParams.get('description')!,
      );
      expect(desc.startsWith('Lab: pixel\\-lab\\-mtv')).toBe(true);
      expect(desc).toContain(
        '| Device ID | Hostname | Run Target | State | Mobile Harness |',
      );
      expect(desc).toContain('pixel-dev-1');
      expect(desc).toContain('pixel-dev-2');
      expect(desc).toContain('[View in MH](');

      const fconLinkPos = desc.indexOf(
        '[View selected devices in Fleet Console]',
      );
      const tablePos = desc.indexOf('| Device ID | Hostname |');
      expect(fconLinkPos).toBeGreaterThan(-1);
      expect(fconLinkPos).toBeLessThan(tablePos);
    });

    it('should disable the button when selected devices are from different labs', () => {
      const dev1 = createPixelDevice('pixel-dev-1', 'lab-alpha');
      const dev2 = createPixelDevice('pixel-dev-2', 'lab-beta');

      render(
        <FakeAuthStateProvider>
          <RequestRepair
            selectedItems={[dev1, dev2]}
            platform={Platform.PIXEL}
          />
        </FakeAuthStateProvider>,
      );

      const button = screen.getByTestId('file-restoration-bug-button');
      expect(button).toBeInTheDocument();
      expect(button).toBeDisabled();

      fireEvent.click(button);
      expect(windowOpenSpy).not.toHaveBeenCalled();
    });

    it('should disable the button when a selected device is missing lab information', () => {
      const dev = createPixelDevice('pixel-dev-no-lab');

      render(
        <FakeAuthStateProvider>
          <RequestRepair selectedItems={[dev]} platform={Platform.PIXEL} />
        </FakeAuthStateProvider>,
      );

      const button = screen.getByTestId('file-restoration-bug-button');
      expect(button).toBeInTheDocument();
      expect(button).toBeDisabled();

      fireEvent.click(button);
      expect(windowOpenSpy).not.toHaveBeenCalled();
    });

    it('should disable the button when multiple selected devices all have missing lab information', () => {
      const dev1 = createPixelDevice('pixel-dev-1');
      const dev2 = createPixelDevice('pixel-dev-2');

      render(
        <FakeAuthStateProvider>
          <RequestRepair
            selectedItems={[dev1, dev2]}
            platform={Platform.PIXEL}
          />
        </FakeAuthStateProvider>,
      );

      const button = screen.getByTestId('file-restoration-bug-button');
      expect(button).toBeInTheDocument();
      expect(button).toBeDisabled();

      fireEvent.click(button);
      expect(windowOpenSpy).not.toHaveBeenCalled();
    });
  });
});
