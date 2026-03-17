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

import { Alert, Box } from '@mui/material';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { COMMON_MESSAGES } from '@/crystal_ball/constants/messages';

interface RequireLoginProps {
  children: React.ReactNode;
}

/**
 * A wrapper component that requires the user to be logged in to view its children.
 * If the user is anonymous, it displays a warning alert instead.
 */
export function RequireLogin({ children }: RequireLoginProps) {
  const authState = useAuthState();
  const isAnonymous = authState.identity === ANONYMOUS_IDENTITY;

  if (isAnonymous) {
    return (
      <Box sx={{ p: 3 }}>
        <Alert severity="warning">{COMMON_MESSAGES.LOGIN_REQUIRED}</Alert>
      </Box>
    );
  }

  return <>{children}</>;
}
