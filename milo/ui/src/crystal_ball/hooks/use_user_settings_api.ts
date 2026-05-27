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
  UseMutationOptions,
  UseMutationResult,
  UseQueryResult,
} from '@tanstack/react-query';

import {
  useGapiMutation,
  useGapiQuery,
} from '@/common/hooks/gapi_query/gapi_query';
import { WrapperQueryOptions } from '@/common/types/query_wrapper_options';
import { API_V1_BASE_PATH as BASE_PATH } from '@/crystal_ball/constants';
import {
  GetUserSettingsRequest,
  UpdateUserSettingsRequest,
  UserSettings,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

/**
 * Returns the query key for GetUserSettings.
 * Useful for cache invalidation.
 */
export const getUserSettingsQueryKey = (name: string) => [
  'gapi',
  'GET',
  `${BASE_PATH}/${name}`,
];

/**
 * Hook for GetUserSettings.
 * @param request - The get request payload.
 * @param options - Optional query options.
 * @returns The UserSettings.
 */
export const useGetUserSettings = (
  request: GetUserSettingsRequest,
  options?: WrapperQueryOptions<UserSettings>,
): UseQueryResult<UserSettings> => {
  return useGapiQuery<UserSettings>(
    {
      path: `${BASE_PATH}/${request.name}`,
      method: 'GET',
    },
    options,
  );
};

/**
 * Hook for UpdateUserSettings.
 * @param options - Optional mutation options.
 * @returns A mutation result for updating user settings.
 */
export const useUpdateUserSettings = (
  options?: UseMutationOptions<UserSettings, Error, UpdateUserSettingsRequest>,
): UseMutationResult<UserSettings, Error, UpdateUserSettingsRequest> => {
  return useGapiMutation<UpdateUserSettingsRequest, UserSettings>(
    (request) => ({
      path: `${BASE_PATH}/${request.userSettings?.name}`,
      method: 'PATCH',
      body: request.userSettings,
      params: {
        updateMask: request.updateMask?.join(','),
      },
    }),
    options,
  );
};
