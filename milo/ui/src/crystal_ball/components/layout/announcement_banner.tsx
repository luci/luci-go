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

import { Close as CloseIcon } from '@mui/icons-material';
import Alert, { AlertColor } from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Collapse from '@mui/material/Collapse';
import IconButton from '@mui/material/IconButton';
import { useMemo, useState } from 'react';

import { useAuthState } from '@/common/components/auth_state_provider';
import { useListAnnouncements } from '@/crystal_ball/hooks';
import {
  Announcement,
  Announcement_Severity,
  Announcement_State,
  Announcement_Visibility,
  ListAnnouncementsRequest,
} from '@/proto/go.chromium.org/luci/crystal_ball/api/perf_service.pb';

function getAlertSeverity(
  severity?: Announcement_Severity | string,
): AlertColor {
  const sevStr =
    typeof severity === 'string'
      ? severity.toUpperCase()
      : Announcement_Severity[severity ?? Announcement_Severity.INFO];
  switch (sevStr) {
    case 'ERROR':
      return 'error';
    case 'WARNING':
      return 'warning';
    case 'INFO':
    default:
      return 'info';
  }
}

function isActiveState(state?: Announcement_State | string): boolean {
  if (state === undefined) {
    return true;
  }
  if (typeof state === 'string') {
    return state.toUpperCase() === 'ACTIVE';
  }
  return state === Announcement_State.ACTIVE;
}

function isVisibleForUser(
  visibility: Announcement_Visibility | string | undefined,
  isInternalUser: boolean,
): boolean {
  if (visibility === undefined) {
    return true;
  }
  const upper =
    typeof visibility === 'string'
      ? visibility.toUpperCase()
      : Announcement_Visibility[visibility];
  if (upper === 'BOTH' || upper === 'VISIBILITY_UNSPECIFIED') {
    return true;
  }
  if (isInternalUser) {
    return upper === 'INTERNAL';
  }
  return upper === 'EXTERNAL';
}

/**
 * AnnouncementBanner fetches and displays active announcements
 * filtered by internal vs external visibility based on user auth state.
 */
export function AnnouncementBanner() {
  const authState = useAuthState();
  const isInternalUser = /@google\.com$/.test(authState.email || '');

  const [dismissedNames, setDismissedNames] = useState<ReadonlySet<string>>(
    new Set(),
  );

  const { data } = useListAnnouncements(
    ListAnnouncementsRequest.fromPartial({
      filter: 'state=ACTIVE',
    }),
  );

  const activeAnnouncements = useMemo(() => {
    const list = data?.announcements || [];
    return list.filter((announcement: Announcement) => {
      if (!isActiveState(announcement.state)) {
        return false;
      }
      if (!isVisibleForUser(announcement.visibility, isInternalUser)) {
        return false;
      }
      return !dismissedNames.has(announcement.name);
    });
  }, [data?.announcements, dismissedNames, isInternalUser]);

  if (activeAnnouncements.length === 0) {
    return null;
  }

  return (
    <Box
      sx={{
        width: '100%',
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
      }}
      role="region"
      aria-label="Service Announcements"
    >
      {activeAnnouncements.map((announcement) => (
        <Collapse key={announcement.name} in>
          <Alert
            severity={getAlertSeverity(announcement.severity)}
            action={
              <IconButton
                aria-label={`Dismiss announcement: ${announcement.message}`}
                color="inherit"
                size="small"
                onClick={() => {
                  setDismissedNames((prev) => {
                    const next = new Set(prev);
                    next.add(announcement.name);
                    return next;
                  });
                }}
              >
                <CloseIcon fontSize="inherit" />
              </IconButton>
            }
            sx={{
              borderRadius: 0,
              borderLeft: 'none',
              borderRight: 'none',
            }}
          >
            {announcement.message}
          </Alert>
        </Collapse>
      ))}
    </Box>
  );
}
