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

import { obtainAuthState } from '@/common/api/auth_state';

import { loadRouteForGooglersOnly } from './routes';

jest.mock('@/common/api/auth_state');
const mockedObtainAuthState = jest.mocked(obtainAuthState);

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
