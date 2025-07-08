// Copyright 2025 The LUCI Authors.
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

import { Box, Card } from '@mui/material';

import { AssociatedCLsSection } from './associated_cls_section'; // New import
import { HistoryRateDisplaySection } from './history_rate_display';
import { OverviewActionsSection } from './overview_actions_section';
import { OverviewStatusSection } from './overview_status_section';

interface OverviewSectionProps {
  expanded: boolean;
}

export function OverviewSection({ expanded }: OverviewSectionProps) {
  return (
    <Box
      sx={{
        flex: expanded ? { md: 3 } : { md: 9 },
      }}
    >
      <Card
        sx={{
          p: 2,
          height: '100%',
          boxSizing: 'border-box',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Box
          sx={{
            display: 'flex',
            flexDirection: expanded ? 'column' : 'row',
            gap: expanded ? '16px' : '40px',
          }}
        >
          <OverviewStatusSection expanded={expanded} />
          <AssociatedCLsSection expanded={expanded} />

          <HistoryRateDisplaySection />

          {expanded && <OverviewActionsSection />}
        </Box>
        {!expanded && (
          <Box sx={{ mt: 'auto' }}>
            <OverviewActionsSection />
          </Box>
        )}
      </Card>
    </Box>
  );
}
