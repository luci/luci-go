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

import { useQuery } from '@tanstack/react-query';
import { useContext } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { extractProject } from '@/common/tools/utils';

import { SemaphoreMessage, fetchSemaphoreMessages } from '../../api/semaphore';
import { ChronicleContext } from '../context';

/**
 * Poll interval matching the 4-minute refresh cadence of Android Build's
 * Semaphore notification bar.
 */
export const SEMAPHORE_POLL_INTERVAL_MS = 4 * 60 * 1000;

const EMPTY_MESSAGES: readonly SemaphoreMessage[] = [];

/**
 * Polls the enabled global Android Build Semaphore messages when the signed-in
 * user is a Googler viewing an Android workplan.
 *
 * @param propRealm Optional workplan security realm (e.g. 'android:presubmit').
 *   Defaults to the realm from `ChronicleContext`.
 */
export function useSemaphoreMessages(
  propRealm?: string,
): readonly SemaphoreMessage[] {
  const { graph } = useContext(ChronicleContext);
  const realm = propRealm ?? graph?.realm;
  const isAndroidRealm = extractProject(realm ?? '') === 'android';
  // Semaphore is behind ÜberProxy, so only Googlers can read it.
  const isGoogler = useAuthState().email?.endsWith('@google.com') ?? false;
  const semaphoreHost = SETTINGS.chronicle?.semaphoreHost;
  const semaphoreApiKey = SETTINGS.chronicle?.semaphoreApiKey;
  const isEnabled =
    isGoogler && isAndroidRealm && Boolean(semaphoreHost && semaphoreApiKey);

  const { data } = useQuery({
    queryKey: ['chronicle', 'semaphore-messages', semaphoreHost],
    queryFn: () => fetchSemaphoreMessages(semaphoreHost!, semaphoreApiKey!),
    refetchInterval: SEMAPHORE_POLL_INTERVAL_MS,
    retry: false,
    enabled: isEnabled,
  });

  return isEnabled ? (data ?? EMPTY_MESSAGES) : EMPTY_MESSAGES;
}
