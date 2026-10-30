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

import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import React from 'react';

import { AuthState } from '@/common/api/auth_state';
import {
  FeatureFlag,
  getFeatureFlagLocalStorageKey,
} from '@/common/feature_flags/context';
import { ShortcutProvider } from '@/fleet/components/shortcut_provider';
import { SettingsProvider } from '@/fleet/context/providers';
import {
  fleetFeedback,
  fleetFilterBar,
  fleetTable,
} from '@/fleet/testing_tools/drivers';
import { fakeFleetConsoleBackend } from '@/fleet/testing_tools/mock_api/fake_fleet_console_backend';
import { fakeGoogleAnalytics } from '@/fleet/testing_tools/mocks/fake_google_analytics';
import { fakeNavigation } from '@/fleet/testing_tools/mocks/fake_navigation';
import { FakeAuthStateProvider } from '@/testing_tools/fakes/fake_auth_state_provider';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';
import { createMockAuthState } from '@/testing_tools/mocks/authstate_mock';

export { fleetFeedback, fleetFilterBar, fleetTable };

export interface DeferredRpcController {
  resolveWith: (data: unknown) => Promise<void>;
  rejectWithError: (grpcCode: number, message: string) => Promise<void>;
}

const nav = fakeNavigation();
let analytics: ReturnType<typeof fakeGoogleAnalytics> | undefined;
let currentAuthState: AuthState = createMockAuthState();
let currentInitialUrl = '/';
let currentMountedPath: string | undefined;
let currentUser: ReturnType<typeof userEvent.setup> = userEvent.setup({
  delay: null,
});

/**
 * Returns the active `userEvent` instance configured for the current test
 * (`{ delay: null }`), initialized automatically in `beforeEach`.
 */
export const getUser = () => currentUser;

/**
 * Registers deterministic Jest `beforeEach` / `afterEach` lifecycle hooks for
 * Fleet Console integration tests (mock pRPC interceptor, fake navigation,
 * fake analytics, localStorage cleanup, and real-timer restoration).
 */
const setupFleetIntegrationTest = () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.localStorage.clear();
    nav.clear();
    currentUser = userEvent.setup({ delay: null });
    currentAuthState = createMockAuthState();
    currentInitialUrl = '/';
    currentMountedPath = undefined;
    analytics = fakeGoogleAnalytics();
    fakeFleetConsoleBackend.reset();
    fakeFleetConsoleBackend.install();
    fakeFleetConsoleBackend.permissions.setDefaultPermission(true);
  });

  afterEach(() => {
    jest.useRealTimers();
    analytics?.clear();
    window.localStorage.clear();
    fakeFleetConsoleBackend.uninstall();
    jest.restoreAllMocks();
  });
};

setupFleetIntegrationTest();

export const genericGiven = {
  permission(hasPermission: boolean) {
    fakeFleetConsoleBackend.permissions.setDefaultPermission(hasPermission);
  },

  featureFlag(flag: FeatureFlag, enabled = true) {
    window.localStorage.setItem(
      getFeatureFlagLocalStorageKey(flag),
      enabled ? 'on' : 'off',
    );
  },

  authenticatedAs(identity: string, email: string) {
    currentAuthState = {
      ...createMockAuthState(),
      identity,
      email,
    };
    fakeFleetConsoleBackend.setAuthState({ identity, email });
  },

  initialUrl(url: string) {
    currentInitialUrl = url;
  },

  mountedPath(path: string) {
    currentMountedPath = path;
  },

  page(pageNumber: number, pageSize: number) {
    currentInitialUrl = fleetTable.pageUrl(pageNumber, pageSize);
  },

  rpcError(method: string, grpcCode: number, message: string) {
    fakeFleetConsoleBackend.setError(method, grpcCode, message);
  },

  rpcResponse(method: string, response: unknown) {
    fakeFleetConsoleBackend.setStaticResponse(method, response);
  },

  deferredRpc(method: string): DeferredRpcController {
    let resolveDeferred!: (value: unknown) => void;
    const pendingPromise = new Promise<unknown>((resolve) => {
      resolveDeferred = resolve;
    });
    fakeFleetConsoleBackend.setDeferredResponse(method, pendingPromise);
    return {
      async resolveWith(data: unknown) {
        await act(async () => {
          resolveDeferred(data);
          await pendingPromise;
        });
      },
      async rejectWithError(grpcCode: number, message: string) {
        await act(async () => {
          resolveDeferred({ __isError: true, grpcCode, message });
          await pendingPromise;
        });
      },
    };
  },
};

export const genericWhen = {
  renderPage(ui: React.ReactElement) {
    return render(
      <FakeContextProvider
        mountedPath={currentMountedPath}
        routerOptions={{ initialEntries: [currentInitialUrl] }}
        siblingRoutes={nav.siblingRoutes}
      >
        <FakeAuthStateProvider value={currentAuthState}>
          <SettingsProvider>
            <ShortcutProvider>
              {nav.observerElement}
              {ui}
            </ShortcutProvider>
          </SettingsProvider>
        </FakeAuthStateProvider>
      </FakeContextProvider>,
    );
  },

  async userClicksButton(name: string | RegExp) {
    const button = await screen.findByRole('button', { name });
    await getUser().click(button);
  },

  async userClicksLink(name: string | RegExp) {
    const link = await screen.findByRole('link', { name });
    await getUser().click(link);
  },

  async timePasses(ms: number) {
    await act(async () => {
      await jest.advanceTimersByTimeAsync(ms);
    });
  },
};

export const genericThen = {
  navigatedTo(expectedPath: string) {
    expect(nav.lastPath).toBe(expectedPath);
  },

  searchParamIs(key: string, expectedValue: string | null) {
    const params = new URLSearchParams(nav.lastSearch ?? '');
    expect(params.get(key)).toBe(expectedValue);
  },

  trackedAnalyticsEvent(eventName: string, payload?: Record<string, unknown>) {
    if (payload) {
      expect(analytics?.events).toContainEqual({ eventName, payload });
    } else {
      expect(analytics?.events.map((e) => e.eventName)).toContain(eventName);
    }
  },

  headingIsVisible(name: string | RegExp, level?: number) {
    expect(screen.getByRole('heading', { name, level })).toBeInTheDocument();
  },

  async textIsVisible(text: string | RegExp) {
    expect(await screen.findByText(text)).toBeInTheDocument();
  },

  textIsNotVisible(text: string | RegExp) {
    expect(screen.queryByText(text)).not.toBeInTheDocument();
  },

  async tableIsEmpty(emptyMessage?: string | RegExp) {
    await fleetTable.expectEmpty(emptyMessage);
  },

  tableColumnsAre(expectedHeaders: string[], container?: HTMLElement) {
    fleetTable.expectColumns(expectedHeaders, container);
  },

  async errorAlertIsVisible(title: string | RegExp, message?: string | RegExp) {
    await fleetFeedback.expectErrorAlert(title, message);
  },

  async waitUntilRemoved(query: () => HTMLElement | null) {
    await waitFor(() => {
      expect(query()).not.toBeInTheDocument();
    });
  },

  rpcWasCalledWith(method: string, expectedPayload: Record<string, unknown>) {
    const calls = fakeFleetConsoleBackend.getCalls(method);
    expect(calls.length).toBeGreaterThan(0);
    expect(calls.map((c) => c.payload)).toContainEqual(
      expect.objectContaining(expectedPayload),
    );
  },

  rpcWasNotCalled(method: string) {
    expect(fakeFleetConsoleBackend.getCalls(method)).toHaveLength(0);
  },
};
