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

import React, { useState, useEffect, useMemo } from 'react';

import {
  useGetUserSettings,
  useUpdateUserSettings,
} from '@/crystal_ball/hooks';
import { UserSettings } from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

import { UserSettingsContext } from './user_settings_context';

const USER_SETTINGS_NAME = 'users/me/settings';

export function UserSettingsProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  // Fetch settings from server
  const { data: settings, isLoading } = useGetUserSettings({
    name: USER_SETTINGS_NAME,
  });

  const updateSettingsMutation = useUpdateUserSettings();

  // State initialized to UTC default, then synchronized with server
  const [localMode, setLocalMode] = useState<'UTC' | 'Local'>('UTC');

  // Sync server settings to local state when loaded
  useEffect(() => {
    if (settings?.timeZone) {
      setLocalMode(settings.timeZone === 'Local' ? 'Local' : 'UTC');
    }
  }, [settings?.timeZone]);

  const activeTimeZone = useMemo(() => {
    if (localMode === 'UTC') {
      return 'UTC';
    }
    // Resolve to browser local timezone
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  }, [localMode]);

  const isLocal = useMemo(() => {
    return localMode === 'Local';
  }, [localMode]);

  const updateTimeZone = async (mode: 'UTC' | 'Local') => {
    // Optimistic update in-memory
    setLocalMode(mode);

    const currentSettings =
      settings ?? UserSettings.fromPartial({ name: USER_SETTINGS_NAME });
    await updateSettingsMutation.mutateAsync({
      userSettings: {
        ...currentSettings,
        timeZone: mode,
      },
      updateMask: ['timeZone'],
      allowMissing: false,
    });
  };

  return (
    <UserSettingsContext.Provider
      value={{
        timeZone: activeTimeZone,
        isLocal,
        updateTimeZone,
        isLoading,
        userSettings: settings,
      }}
    >
      {children}
    </UserSettingsContext.Provider>
  );
}
