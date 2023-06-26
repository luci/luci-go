import { useCallback } from 'react';
import { useLatest } from 'react-use';

import { AuthState } from '@/common/api/auth_state';
import { AuthStateContext } from '@/common/components/auth_state_provider';
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
  const getAuthState = useCallback(() => valueRef.current, [value.identity]);

  return (
    <AuthStateContext.Provider value={getAuthState}>
      {children}
    </AuthStateContext.Provider>
  );
};
