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

import { useAuthState } from '@/common/components/auth_state_provider';

/**
 * Fallback used when the auth state carries neither an email nor an
 * identity (e.g. in tests). Never matches a real `claimed_by` value.
 */
export const UNKNOWN_USER_EMAIL = 'current_user';

/**
 * Returns the email of the logged in user as the repair queue backends store
 * it in `claimed_by`: the auth state email, falling back to the identity with
 * its `user:` prefix stripped, and finally to UNKNOWN_USER_EMAIL.
 */
export const useCurrentUserEmail = (): string => {
  const authState = useAuthState();
  const email = authState.email?.trim();
  const rawIdentity = authState.identity?.trim();
  const identity = rawIdentity?.startsWith('user:')
    ? rawIdentity.replace(/^user:/, '')
    : rawIdentity;
  return email || identity || UNKNOWN_USER_EMAIL;
};
