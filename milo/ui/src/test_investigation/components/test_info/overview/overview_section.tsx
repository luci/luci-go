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

export function OverviewSection() {
  return (
    <Box sx={{ flex: { md: 1 }, minWidth: { md: 300 } }}>
      <Card
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: '32px',
          p: 3,
          height: '100%',
          boxSizing: 'border-box',
        }}
      >
        <OverviewStatusSection />

        <HistoryRateDisplaySection />

        <AssociatedCLsSection />

        <OverviewActionsSection />
      </Card>
    </Box>
  );
}
