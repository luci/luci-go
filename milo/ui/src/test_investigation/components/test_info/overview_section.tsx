// src/test_investigation/components/test_info/overview_section.tsx
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

import { Box, Card, Divider, Typography } from '@mui/material';
import React, { JSX } from 'react';

import { AssociatedBug } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';
import { TestAnalysis } from '@/proto/go.chromium.org/luci/bisection/proto/v1/analyses.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import {
  FormattedCLInfo,
  SegmentAnalysisResult,
} from '../../utils/test_info_utils';

import { OverviewActionsSection } from './overview_actions_section';
import { OverviewHistorySection } from './overview_history_section';
import { OverviewStatusSection } from './overview_status_section';

interface OverviewSectionProps {
  invocation: Invocation; // Pass full invocation for project & actions
  testVariant: TestVariant; // Pass full testVariant for status & actions
  associatedBugs?: readonly AssociatedBug[];
  bisectionAnalysis?: TestAnalysis | null;
  segmentAnalysis: SegmentAnalysisResult;
  allFormattedCLs: readonly FormattedCLInfo[];
  onCLPopoverOpen: (event: React.MouseEvent<HTMLElement>) => void;

  allTestHistoryLink: string;
  clHistoryLinkSuffix: string;
  compareLink: string;
}

export function OverviewSection({
  invocation,
  testVariant,
  associatedBugs,
  bisectionAnalysis,
  segmentAnalysis,
  allFormattedCLs,
  onCLPopoverOpen,
  allTestHistoryLink,
  clHistoryLinkSuffix,
  compareLink,
}: OverviewSectionProps): JSX.Element {
  const project = invocation.realm?.split(':')[0];

  return (
    <Box sx={{ flex: { md: 1 }, minWidth: { md: 300 } }}>
      <Card
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 1.5,
          p: 2,
          height: '100%',
        }}
      >
        <Typography variant="h6" component="div" sx={{ mb: 0.5 }}>
          Overview
        </Typography>

        <OverviewStatusSection
          testVariantStatus={testVariant.status}
          associatedBugs={associatedBugs}
          bisectionAnalysis={bisectionAnalysis}
          project={project}
        />

        <Divider sx={{ my: 1 }} />

        <OverviewHistorySection
          segmentAnalysis={segmentAnalysis}
          allTestHistoryLink={allTestHistoryLink}
          allFormattedCLs={allFormattedCLs}
          onCLPopoverOpen={onCLPopoverOpen}
          clHistoryLinkSuffix={clHistoryLinkSuffix}
        />

        <OverviewActionsSection
          invocation={invocation}
          testVariant={testVariant}
          compareLink={compareLink}
        />
      </Card>
    </Box>
  );
}
