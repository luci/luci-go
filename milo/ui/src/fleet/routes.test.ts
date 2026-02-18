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

import { obtainAuthState } from '@/common/api/auth_state';
import * as surveyUtils from '@/fleet/utils/survey';
import { trackedRedirect } from '@/generic_libs/tools/react_router_utils/route_utils';

import { fleetRoutes, loadRouteForGooglersOnly } from './routes';

jest.mock('@/generic_libs/tools/react_router_utils/route_utils');
const mockedTrackedRedirect = jest.mocked(trackedRedirect);

jest.mock('@/common/api/auth_state');
const mockedObtainAuthState = jest.mocked(obtainAuthState);

jest.mock('@/fleet/utils/survey');
const mockedInitiateSurvey = jest.mocked(surveyUtils.initiateSurvey);

describe('loadRouteForGooglersOnly', () => {
  let onSuccess: jest.Mock;
  let onFail: jest.Mock;

  beforeEach(() => {
    jest.resetAllMocks();
    onSuccess = jest.fn(() => Promise.resolve({ component: 'Success' }));
    onFail = jest.fn(() => Promise.resolve({ component: 'Fail' }));
  });

  it('should call onSuccess for a Googler', async () => {
    mockedObtainAuthState.mockResolvedValue({
      identity: 'user:test@google.com',
      email: 'test@google.com',
    });
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail);
    await expect(lazyLoader()).resolves.toEqual({ component: 'Success' });

    expect(onSuccess).toHaveBeenCalledTimes(1);
    expect(onFail).not.toHaveBeenCalled();
    expect(mockedObtainAuthState).toHaveBeenCalledTimes(1);
  });

  it('should call onFail for a non-Googler', async () => {
    mockedObtainAuthState.mockResolvedValue({
      identity: 'user:test@example.com',
      email: 'test@example.com',
    });
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail);
    await expect(lazyLoader()).resolves.toEqual({ component: 'Fail' });

    expect(onFail).toHaveBeenCalledTimes(1);
    expect(onSuccess).not.toHaveBeenCalled();
    expect(mockedObtainAuthState).toHaveBeenCalledTimes(1);
  });

  it('should call onFail for an anonymous user', async () => {
    mockedObtainAuthState.mockResolvedValue({
      identity: 'anonymous:anonymous',
      email: undefined,
    });
    const lazyLoader = loadRouteForGooglersOnly(onSuccess, onFail);
    await expect(lazyLoader()).resolves.toEqual({ component: 'Fail' });

    expect(onFail).toHaveBeenCalledTimes(1);
    expect(onSuccess).not.toHaveBeenCalled();
    expect(mockedObtainAuthState).toHaveBeenCalledTimes(1);
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
});
