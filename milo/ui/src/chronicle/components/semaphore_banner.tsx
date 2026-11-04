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
import { Alert, AlertColor, IconButton } from '@mui/material';
import { useMemo, useState } from 'react';

import { LinkifiedText } from '@/common/components/linkified_text';

import { useSemaphoreMessages } from './hooks/use_semaphore_messages';

function getAlertSeverity(urgency?: string): AlertColor {
  switch (urgency) {
    case 'FAIL':
      return 'error';
    case 'INFO':
      return 'info';
    case 'WARN':
    default:
      return 'warning';
  }
}

export interface SemaphoreBannerProps {
  /**
   * The security realm of the current workplan (e.g. 'android:presubmit').
   * If omitted, reads the realm from `ChronicleContext`.
   */
  readonly realm?: string;
}

/** Shows the enabled global Android Build Semaphore notifications for Android workplans. */
export function SemaphoreBanner({ realm }: SemaphoreBannerProps = {}) {
  const messages = useSemaphoreMessages(realm);
  const [dismissed, setDismissed] = useState<ReadonlySet<string>>(new Set());

  const shown = useMemo(
    () => messages.filter((msg) => !dismissed.has(msg.name)),
    [messages, dismissed],
  );

  const alerts = useMemo(
    () =>
      shown.map((msg) => {
        const text = msg.longMessage || msg.shortMessage || '';
        return (
          <Alert
            key={msg.name}
            severity={getAlertSeverity(msg.urgency)}
            sx={{ borderRadius: 0 }}
            action={
              <IconButton
                aria-label="Dismiss Semaphore notification"
                color="inherit"
                size="small"
                onClick={() =>
                  setDismissed((prev) => new Set(prev).add(msg.name))
                }
              >
                <CloseIcon fontSize="inherit" />
              </IconButton>
            }
          >
            <LinkifiedText text={text} />
          </Alert>
        );
      }),
    [shown],
  );

  if (shown.length === 0) {
    return null;
  }

  return (
    <div role="region" aria-label="System Notifications">
      {alerts}
    </div>
  );
}
