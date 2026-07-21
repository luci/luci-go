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

import { useGapiQuery } from '@/common/hooks/gapi_query/gapi_query';
import { API_V1_BASE_PATH } from '@/crystal_ball/constants';
import {
  ListAnnouncementsRequest,
  ListAnnouncementsResponse,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';
import { FakeContextProvider } from '@/testing_tools/fakes/fake_context_provider';

import {
  getListAnnouncementsQueryKey,
  useListAnnouncements,
} from './use_announcements_api';

jest.mock('@/common/hooks/gapi_query/gapi_query', () => ({
  useGapiQuery: jest.fn(),
  useGapiMutation: jest.fn(),
}));

const mockedUseGapiQuery = jest.mocked(useGapiQuery);

describe('use_announcements_api', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('getListAnnouncementsQueryKey', () => {
    it('returns the GAPI query key format', () => {
      const request = ListAnnouncementsRequest.fromPartial({
        filter: 'state=ACTIVE',
      });

      expect(getListAnnouncementsQueryKey(request)).toEqual([
        'gapi',
        'GET',
        `${API_V1_BASE_PATH}/announcements`,
        { filter: 'state=ACTIVE' },
        undefined,
      ]);
    });
  });

  describe('useListAnnouncements', () => {
    it('should call useGapiQuery with correct path and params', () => {
      const sampleResponse = ListAnnouncementsResponse.fromPartial({
        announcements: [],
      });

      mockedUseGapiQuery.mockReturnValue(
        Object.assign({
          data: sampleResponse,
          isLoading: false,
        }),
      );

      const request = ListAnnouncementsRequest.fromPartial({
        filter: 'state=ACTIVE visibility=INTERNAL',
      });

      const { result } = renderHook(() => useListAnnouncements(request), {
        wrapper: FakeContextProvider,
      });

      expect(mockedUseGapiQuery).toHaveBeenCalledWith(
        {
          path: `${API_V1_BASE_PATH}/announcements`,
          method: 'GET',
          params: {
            filter: 'state=ACTIVE visibility=INTERNAL',
          },
        },
        undefined,
      );
      expect(result.current.data).toEqual(sampleResponse);
    });
  });
});
