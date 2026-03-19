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

import { renderHook } from '@testing-library/react';

import * as gapiQueryHooks from '@/common/hooks/gapi_query/gapi_query';
import { API_V1_BASE_PATH as BASE_PATH } from '@/crystal_ball/constants';
import {
  useGetUserSettings,
  useUpdateUserSettings,
  useAddStarredDashboard,
  useRemoveStarredDashboard,
} from '@/crystal_ball/hooks';
import {
  AddStarredDashboardRequest,
  GetUserSettingsRequest,
  RemoveStarredDashboardRequest,
  UpdateUserSettingsRequest,
  UserSettings,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

// Mock the imported hooks
jest.mock('@/common/hooks/gapi_query/gapi_query', () => ({
  useGapiQuery: jest.fn(),
  useGapiMutation: jest.fn(),
}));

const mockedUseGapiQuery = gapiQueryHooks.useGapiQuery as jest.Mock;
const mockedUseGapiMutation = gapiQueryHooks.useGapiMutation as jest.Mock;

describe('use_user_settings_api', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  const sampleUserSettings: UserSettings = UserSettings.fromPartial({
    name: 'users/me/settings',
    starredDashboards: [],
    timeZone: 'UTC',
  });

  describe('useGetUserSettings', () => {
    const request = GetUserSettingsRequest.fromPartial({
      name: 'users/me/settings',
    });

    it('should call useGapiQuery with correct arguments', () => {
      mockedUseGapiQuery.mockReturnValue({
        data: sampleUserSettings,
        isLoading: false,
      });

      renderHook(() => useGetUserSettings(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledTimes(1);
      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${BASE_PATH}/${request.name}`,
          method: 'GET',
        },
        undefined,
      );
    });
  });

  describe('useUpdateUserSettings', () => {
    const request = UpdateUserSettingsRequest.fromPartial({
      userSettings: sampleUserSettings,
      updateMask: ['timeZone'],
    });

    it('should call useGapiMutation with correct arguments', () => {
      mockedUseGapiMutation.mockReturnValue({
        mutateAsync: jest.fn().mockResolvedValue(sampleUserSettings),
        isPending: false,
      });

      renderHook(() => useUpdateUserSettings(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}/${sampleUserSettings.name}`,
        method: 'PATCH',
        body: request.userSettings,
        params: {
          updateMask: 'timeZone',
        },
      });
    });
  });

  describe('useAddStarredDashboard', () => {
    const request = AddStarredDashboardRequest.fromPartial({
      name: 'users/me/settings',
      dashboard: 'dashboardStates/test-id',
    });

    it('should call useGapiMutation with correct arguments', () => {
      mockedUseGapiMutation.mockReturnValue({
        mutateAsync: jest.fn().mockResolvedValue(sampleUserSettings),
        isPending: false,
      });

      renderHook(() => useAddStarredDashboard(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}/${request.name}:addStarredDashboard`,
        method: 'POST',
        body: {
          dashboard: request.dashboard,
        },
        params: {},
      });
    });
  });

  describe('useRemoveStarredDashboard', () => {
    const request = RemoveStarredDashboardRequest.fromPartial({
      name: 'users/me/settings',
      dashboard: 'dashboardStates/test-id',
    });

    it('should call useGapiMutation with correct arguments', () => {
      mockedUseGapiMutation.mockReturnValue({
        mutateAsync: jest.fn().mockResolvedValue(sampleUserSettings),
        isPending: false,
      });

      renderHook(() => useRemoveStarredDashboard(), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiMutation).toHaveBeenCalledTimes(1);

      const requestBuilder = mockedUseGapiMutation.mock.calls[0][0];
      const builtRequest = requestBuilder(request);

      expect(builtRequest).toEqual({
        path: `${BASE_PATH}/${request.name}:removeStarredDashboard`,
        method: 'POST',
        body: {
          dashboard: request.dashboard,
        },
        params: {},
      });
    });
  });
});
