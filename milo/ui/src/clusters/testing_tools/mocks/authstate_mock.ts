// Copyright 2022 The LUCI Authors.
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

import fetchMock from 'fetch-mock-jest';

export const createMockAuthState = () => {
  return {
    identity: 'user:user@example.com',
    email: 'user@example.com',
    picture: '',
    accessToken: 'token_text_access',
    accessTokenExpiry: 1648105586,
    idToken: 'token_text',
    idTokenExpiry: 1648105586,
  };
};

export const mockFetchAuthState = () => {
  fetchMock.get('/auth/openid/state', createMockAuthState());
};
