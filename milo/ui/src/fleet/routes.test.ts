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

import { LoaderFunction, LoaderFunctionArgs } from 'react-router';

import {
  getAuthStateCache,
  obtainAuthState,
  queryAuthState,
  setAuthStateCache,
} from '@/common/api/auth_state';
import * as surveyUtils from '@/fleet/utils/survey';
import { trackedRedirect } from '@/generic_libs/tools/react_router_utils/route_utils';

import {
  fleetRoutes,
  loadRouteForGooglersOnly,
  resetAnonymousAuthCheckForTesting,
} from './routes';

jest.mock('@/generic_libs/tools/react_router_utils/route_utils');
const mockedTrackedRedirect = jest.mocked(trackedRedirect);

jest.mock('@/common/api/auth_state');
const mockedGetAuthStateCache = jest.mocked(getAuthStateCache);
const mockedObtainAuthState = jest.mocked(obtainAuthState);
const mockedQueryAuthState = jest.mocked(queryAuthState);
const mockedSetAuthStateCache = jest.mocked(setAuthStateCache);

jest.mock('@/fleet/utils/survey');
const mockedInitiateSurvey = jest.mocked(surveyUtils.initiateSurvey);

describe('loadRouteForGooglersOnly', () => {
  let onSuccess: jest.Mock;
  let onFail: jest.Mock;
  let onAnonymous: jest.Mock;

  beforeEach(() => {
    jest.resetAllMocks();
    resetAnonymousAuthCheckForTesting();
    onSuccess = jest.fn(() => Promise.resolve({ component: 'Success' }));
    onFail = jest.fn(() => Promise.resolve({ component: 'Fail' }));
    onAnonymous = jest.fn(() => Promise.resolve({ component: 'SignIn' }));
    mockedGetAuthStateCache.mockResolvedValue(null);
    mockedSetAuthStateCache.mockResolvedValue(undefined);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('should call onSuccess for a Googler', async () => {
    mockedObtainAuthState.mockResolvedValue({
      identity: 'user:test@google.com',
      email: 'test@google.com',
    });
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail, onAnonymous);
    await expect(lazyLoader()).resolves.toEqual({ component: 'Success' });

    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(onFail).not.toHaveBeenCalled();
    expect(onAnonymous).not.toHaveBeenCalled();
    expect(mockedObtainAuthState).toHaveBeenCalledTimes(1);
    expect(mockedQueryAuthState).not.toHaveBeenCalled();
  });

  it('should call onFail for a signed-in non-Googler', async () => {
    mockedObtainAuthState.mockResolvedValue({
      identity: 'user:test@example.com',
      email: 'test@example.com',
    });
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail, onAnonymous);
    await expect(lazyLoader()).resolves.toEqual({ component: 'Fail' });

    expect(onFail).toHaveBeenCalledTimes(1);
    expect(onSuccess).not.toHaveBeenCalled();
    expect(onAnonymous).not.toHaveBeenCalled();
    expect(mockedObtainAuthState).toHaveBeenCalledTimes(1);
    expect(mockedQueryAuthState).not.toHaveBeenCalled();
  });

  it('should call onAnonymous on cold start without duplicate queryAuthState fetch', async () => {
    mockedGetAuthStateCache.mockResolvedValue(null);
    mockedObtainAuthState.mockResolvedValue({
      identity: 'anonymous:anonymous',
      email: undefined,
    });
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail, onAnonymous);
    await expect(lazyLoader()).resolves.toEqual({ component: 'SignIn' });

    expect(onAnonymous).toHaveBeenCalledTimes(1);
    expect(onFail).not.toHaveBeenCalled();
    expect(onSuccess).not.toHaveBeenCalled();
    expect(mockedObtainAuthState).toHaveBeenCalledTimes(1);
    expect(mockedQueryAuthState).not.toHaveBeenCalled();
  });

  it('should refresh stale cached anonymous state once per session and tolerate cache write errors', async () => {
    mockedGetAuthStateCache.mockResolvedValue({
      identity: 'anonymous:anonymous',
      email: undefined,
    });
    const refreshedState = {
      identity: 'user:test@google.com',
      email: 'test@google.com',
    };
    mockedQueryAuthState.mockResolvedValue(refreshedState);
    mockedSetAuthStateCache.mockRejectedValueOnce(new Error('IndexedDB full'));
    jest.spyOn(console, 'warn').mockImplementation(() => {});

    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail, onAnonymous);
    await expect(lazyLoader()).resolves.toEqual({ component: 'Success' });

    expect(mockedQueryAuthState).toHaveBeenCalledTimes(1);
    expect(mockedSetAuthStateCache).toHaveBeenCalledWith(refreshedState);
    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(onAnonymous).not.toHaveBeenCalled();
    expect(onFail).not.toHaveBeenCalled();
  });

  it('should not re-query server on subsequent client-side transitions after verifying anonymous state', async () => {
    mockedGetAuthStateCache.mockResolvedValue({
      identity: 'anonymous:anonymous',
      email: undefined,
    });
    mockedQueryAuthState.mockResolvedValue({
      identity: 'anonymous:anonymous',
      email: undefined,
    });
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail, onAnonymous);
    await expect(lazyLoader()).resolves.toEqual({ component: 'SignIn' });
    await expect(lazyLoader()).resolves.toEqual({ component: 'SignIn' });

    expect(mockedQueryAuthState).toHaveBeenCalledTimes(1);
    expect(mockedObtainAuthState).not.toHaveBeenCalled();
    expect(onAnonymous).toHaveBeenCalledTimes(2);
  });

  it('should call onFail when auth state fetch fails instead of showing sign-in prompt', async () => {
    mockedObtainAuthState.mockRejectedValue(new Error('network error'));
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail, onAnonymous);
    await expect(lazyLoader()).resolves.toEqual({ component: 'Fail' });

    expect(onFail).toHaveBeenCalledTimes(1);
    expect(onAnonymous).not.toHaveBeenCalled();
    expect(onSuccess).not.toHaveBeenCalled();
  });
});

describe('devices loader', () => {
  beforeEach(() => {
    jest.resetAllMocks();
  });

  it('should initiate a survey when the route loads', async () => {
    // Find the parent route object for '/devices'.
    // New structure: root -> children -> p/:platform -> children -> devices
    const devicesRoute = fleetRoutes[0].children
      ?.find((r) => r.path === 'p/:platform')
      ?.children?.find((r) => r.path === 'devices');

    expect(devicesRoute).toBeDefined();
    expect(devicesRoute?.loader).toBeInstanceOf(Function);

    // Execute the loader.
    if (typeof devicesRoute?.loader === 'function') {
      await devicesRoute.loader({} as never, {} as never);
    } else {
      throw new Error('devicesRoute.loader is not a function');
    }

    // Ensure the survey helper was called.
    expect(mockedInitiateSurvey).toHaveBeenCalledTimes(1);
    expect(mockedInitiateSurvey).toHaveBeenCalledWith(
      SETTINGS.fleetConsole.hats,
    );
  });
});

describe('legacy labs redirects', () => {
  const TEST_ORIGIN = 'http://test.local';

  it('should redirect /labs/p/android/devices to /p/android/devices', async () => {
    const labsRoute = fleetRoutes[0].children?.find((r) => r.path === 'labs');
    const pRoute = labsRoute?.children?.find((r) => r.path === '*');

    expect(pRoute).toBeDefined();
    const loader = pRoute?.loader as LoaderFunction;

    const request = new Request(
      `${TEST_ORIGIN}/ui/fleet/labs/p/android/devices`,
    );
    await loader({ request, params: {} } as LoaderFunctionArgs);

    expect(mockedTrackedRedirect).toHaveBeenCalledWith(
      expect.objectContaining({
        from: '/ui/fleet/labs/p/android/devices',
        to: '/ui/fleet/p/android/devices',
      }),
    );
  });

  it('should redirect /labs/devices to /devices', async () => {
    const labsRoute = fleetRoutes[0].children?.find((r) => r.path === 'labs');
    const devicesRoute = labsRoute?.children?.find(
      (r) => r.path === 'devices/:id?',
    );

    expect(devicesRoute).toBeDefined();
    const loader = devicesRoute?.loader as LoaderFunction;

    const request = new Request(`${TEST_ORIGIN}/ui/fleet/labs/devices`);
    await loader({ request, params: {} } as LoaderFunctionArgs);

    expect(mockedTrackedRedirect).toHaveBeenCalledWith(
      expect.objectContaining({
        from: '/ui/fleet/labs/devices',
        to: '/ui/fleet/p/chromeos/devices',
      }),
    );
  });

  it('should preserve query parameters during redirect', async () => {
    const labsRoute = fleetRoutes[0].children?.find((r) => r.path === 'labs');
    const pRoute = labsRoute?.children?.find((r) => r.path === '*');

    expect(pRoute).toBeDefined();
    const loader = pRoute?.loader as LoaderFunction;

    const request = new Request(
      `${TEST_ORIGIN}/ui/fleet/labs/p/android/devices?foo=bar&baz=qux`,
    );
    await loader({ request, params: {} } as LoaderFunctionArgs);

    expect(mockedTrackedRedirect).toHaveBeenCalledWith(
      expect.objectContaining({
        from: '/ui/fleet/labs/p/android/devices?foo=bar&baz=qux',
        to: '/ui/fleet/p/android/devices?foo=bar&baz=qux',
      }),
    );
  });

  it('should redirect arbitrary labs paths to safe paths', async () => {
    const labsRoute = fleetRoutes[0].children?.find((r) => r.path === 'labs');
    const catchAllRoute = labsRoute?.children?.find((r) => r.path === '*');

    expect(catchAllRoute).toBeDefined();
    const loader = catchAllRoute?.loader as LoaderFunction;

    const request = new Request(
      `${TEST_ORIGIN}/ui/fleet/labs/some-future-page`,
    );
    await loader({ request, params: {} } as LoaderFunctionArgs);

    expect(mockedTrackedRedirect).toHaveBeenCalledWith(
      expect.objectContaining({
        from: '/ui/fleet/labs/some-future-page',
        to: '/ui/fleet/some-future-page',
      }),
    );
  });

  it('should redirect /labs (no trailing slash) correctly', async () => {
    const labsRoute = fleetRoutes[0].children?.find((r) => r.path === 'labs');
    const catchAllRoute = labsRoute?.children?.find((r) => r.path === '*');

    expect(catchAllRoute).toBeDefined();
    const loader = catchAllRoute?.loader as LoaderFunction;

    const request = new Request(`${TEST_ORIGIN}/ui/fleet/labs`);
    await loader({ request, params: {} } as LoaderFunctionArgs);

    expect(mockedTrackedRedirect).toHaveBeenCalledWith(
      expect.objectContaining({
        from: '/ui/fleet/labs',
        to: '/ui/fleet',
      }),
    );
  });

  it('registers the top-level sites route', () => {
    const sitesRoute = fleetRoutes[0].children?.find((r) => r.path === 'sites');
    expect(sitesRoute).toBeDefined();
    expect(sitesRoute?.lazy).toBeInstanceOf(Function);
  });
});
