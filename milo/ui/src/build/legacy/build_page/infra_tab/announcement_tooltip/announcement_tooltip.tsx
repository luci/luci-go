// Copyright 2024 The LUCI Authors.
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

import { Box, Button } from '@mui/material';
import { useRef } from 'react';
import { useClickAway, useLocalStorage, useTimeout } from 'react-use';

import { HtmlTooltip } from '@/common/components/html_tooltip';
import { useSetShowPageConfig } from '@/common/components/page_config_state_provider';

const HIDE_NEW_INFRA_TAB_NOTIFICATION_KEY =
  'hide-new-infra-tab-notification-v2';

export interface InfraTabAnnouncementTooltipProps {
  readonly children: JSX.Element;
}

export function InfraTabAnnouncementTooltip({
  children,
}: InfraTabAnnouncementTooltipProps) {
  const [hideInfraTab = false, setHideInfraTab] = useLocalStorage<boolean>(
    HIDE_NEW_INFRA_TAB_NOTIFICATION_KEY,
  );

  // TODO(weiweilin): we need a mechanism to display announcement tooltips
  // sequentially. Currently, other tooltips will simply overwrite this one.
  const tooltipRef = useRef<HTMLDivElement>(null);
  const [stayedLongEnough] = useTimeout(5000);
  useClickAway(tooltipRef, () => {
    if (stayedLongEnough()) {
      setHideInfraTab(true);
    }
  });

  const setShowPageConfig = useSetShowPageConfig();

  return (
    <HtmlTooltip
      open={!hideInfraTab}
      arrow
      title={
        <Box
          ref={tooltipRef}
          onClick={(e) => e.stopPropagation()}
          sx={{
            padding: '10px 5px 10px 5px',
            maxWidth: '500px',
          }}
        >
          <p>Infra details & build steps have been moved to the infra tab.</p>
          <p>
            If you still want to see these details by default, you can use the
            page settings menu on the top-right corner to change the default tab
            to the infra tab.
          </p>
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: '1fr auto auto',
              marginTop: '20px',
            }}
          >
            <Box />
            <Button onClick={() => setHideInfraTab(true)} size="small">
              Dismiss
            </Button>
            <Button
              disabled={setShowPageConfig === null}
              onClick={() => {
                setShowPageConfig?.(true);
                setHideInfraTab(true);
              }}
              variant="contained"
              size="small"
            >
              Change Default Tab
            </Button>
          </Box>
        </Box>
      }
    >
      {children}
    </HtmlTooltip>
  );
}
