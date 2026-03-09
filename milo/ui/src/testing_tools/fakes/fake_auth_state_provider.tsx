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

import { useMemo } from 'react';
import { useLatest } from 'react-use';

import { AuthState } from '@/common/api/auth_state';
import { TestAuthStateContext } from '@/common/components/auth_state_provider';
import { createMockAuthState } from '@/testing_tools/mocks/authstate_mock';

interface props {
  value?: AuthState;
  children: React.ReactElement;
}

export const FakeAuthStateProvider = ({ value, children }: props) => {
  if (!value) {
    value = createMockAuthState();
  }
  const valueRef = useLatest(value);
  const ctxValue = useMemo(
    () => ({
      getAuthState: () => valueRef.current,
      // This doesn't ensure the tokens are valid. But it should be good
      // enough for most unit tests that is not testing <AuthStateProvider />
      // itself.
      getAccessToken: async () => valueRef.current.accessToken || '',
      getIdToken: async () => valueRef.current.idToken || '',
    }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [value.identity],
  );

  return (
    <TestAuthStateContext.Provider value={ctxValue}>
      {children}
    </TestAuthStateContext.Provider>
  );
};
