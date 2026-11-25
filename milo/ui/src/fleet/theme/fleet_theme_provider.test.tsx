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

import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

import { SettingsMenu } from '../layouts/settings_menu';

import { FLEET_THEME_STORAGE_KEY } from './fleet_theme_context';
import { FleetThemeProvider } from './fleet_theme_provider';
import { ThemeToggleButton } from './theme_toggle_button';

const mockTrackEvent = jest.fn();

jest.mock('@/fleet/hooks/use_fleet_analytics', () => ({
  useFleetAnalytics: () => ({
    trackEvent: mockTrackEvent,
    trackExternalLinkClick: jest.fn(),
  }),
}));

jest.mock('@/common/components/version_control', () => ({
  VersionControlIcon: () => <span />,
  useSwitchVersion: () => jest.fn(),
  ROLLBACK_DURATION_WEEK: 2,
}));

jest.mock('@/common/components/version_info', () => ({
  VersionInfo: () => <li>Version: test</li>,
}));

function setPrefersDark(matches: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: jest.fn().mockImplementation((query: string) => ({
      matches: query === '(prefers-color-scheme: dark)' ? matches : false,
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
}

describe('FleetThemeProvider and theme controls', () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.sessionStorage.clear();
    delete document.documentElement.dataset.fleetTheme;
    mockTrackEvent.mockClear();
    window.gtag = jest.fn();
    setPrefersDark(false);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('defaults to light without writing localStorage and tracks default session once', () => {
    render(
      <MemoryRouter>
        <FleetThemeProvider>
          <ThemeToggleButton />
        </FleetThemeProvider>
      </MemoryRouter>,
    );

    expect(document.documentElement.dataset.fleetTheme).toBe('light');
    expect(window.localStorage.getItem(FLEET_THEME_STORAGE_KEY)).toBeNull();
    expect(mockTrackEvent).toHaveBeenCalledWith('theme_session_active', {
      theme: 'light',
      themeChoice: 'default',
      themeResolved: 'light',
    });
    expect(window.gtag).toHaveBeenCalledWith('set', 'user_properties', {
      fleet_theme: 'light',
      fleet_theme_choice: 'default',
    });
  });

  it('toggles light <-> dark from ThemeToggleButton and logs matching theme telemetry', () => {
    render(
      <MemoryRouter>
        <FleetThemeProvider>
          <ThemeToggleButton />
        </FleetThemeProvider>
      </MemoryRouter>,
    );

    fireEvent.click(
      screen.getByRole('button', { name: 'Switch to dark mode' }),
    );

    expect(document.documentElement.dataset.fleetTheme).toBe('dark');
    expect(window.localStorage.getItem(FLEET_THEME_STORAGE_KEY)).toBe('dark');
    expect(mockTrackEvent).toHaveBeenCalledWith('theme_changed', {
      componentName: 'header_toggle',
      theme: 'dark',
      themeChoice: 'dark',
      themeResolved: 'dark',
    });

    fireEvent.click(
      screen.getByRole('button', { name: 'Switch to light mode' }),
    );
    expect(document.documentElement.dataset.fleetTheme).toBe('light');
    expect(window.localStorage.getItem(FLEET_THEME_STORAGE_KEY)).toBe('light');
  });

  it('exposes keyboard-reachable menuitemradio choices in SettingsMenu including Auto', () => {
    setPrefersDark(true);

    render(
      <MemoryRouter>
        <FleetThemeProvider>
          <SettingsMenu />
        </FleetThemeProvider>
      </MemoryRouter>,
    );

    fireEvent.click(screen.getByRole('button', { name: 'Open menu' }));
    const autoItem = screen.getByRole('menuitemradio', {
      name: /Auto \(system\)/i,
    });
    expect(autoItem.getAttribute('aria-checked')).toBe('false');

    fireEvent.click(autoItem);

    expect(document.documentElement.dataset.fleetTheme).toBe('dark');
    expect(window.localStorage.getItem(FLEET_THEME_STORAGE_KEY)).toBe('auto');
    expect(mockTrackEvent).toHaveBeenCalledWith('theme_changed', {
      componentName: 'settings_menu',
      theme: 'dark',
      themeChoice: 'auto',
      themeResolved: 'dark',
    });
  });

  it('handles blocked sessionStorage gracefully without crashing', () => {
    jest
      .spyOn(Storage.prototype, 'getItem')
      .mockImplementation((key: string) => {
        if (key === 'fleet-console-theme-session-tracked') {
          throw new DOMException('Blocked', 'SecurityError');
        }
        return null;
      });

    expect(() =>
      render(
        <MemoryRouter>
          <FleetThemeProvider>
            <ThemeToggleButton />
          </FleetThemeProvider>
        </MemoryRouter>,
      ),
    ).not.toThrow();
    expect(document.documentElement.dataset.fleetTheme).toBe('light');
  });
});
