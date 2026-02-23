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

import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import { Box, Tab, Tabs, Tooltip, IconButton } from '@mui/material';
import { useRef, useState } from 'react';

import { OutputTestVerdict } from '@/common/types/verdict';
import { AnyInvocation } from '@/test_investigation/utils/invocation_utils';

import { AggregationView } from './aggregation_view/aggregation_view';
import { TestAggregationProvider } from './context/provider';
import { TestAggregationToolbar } from './test_aggregation_toolbar';
import { TriageView } from './triage_view';

export interface TestAggregationViewerProps {
  initialExpandedIds?: string[];
  invocation: AnyInvocation;
  testVariant?: OutputTestVerdict;
  autoLocate?: boolean;
  defaultExpanded?: boolean;
}

export interface TestInvestigationViewHandle {
  locateCurrentTest: () => void;
}

export function TestAggregationViewer(props: TestAggregationViewerProps) {
  const { testVariant } = props;
  const [activeTab, setActiveTab] = useState(0);
  const activeViewRef = useRef<TestInvestigationViewHandle>(null);

  const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
    setActiveTab(newValue);
  };

  const handleLocateCurrentTest = () => {
    activeViewRef.current?.locateCurrentTest();
  };

  return (
    <TestAggregationProvider>
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
        <TestAggregationToolbar
          onLocateCurrentTest={
            testVariant ? handleLocateCurrentTest : undefined
          }
        />
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs
            value={activeTab}
            onChange={handleTabChange}
            aria-label="test aggregation views"
            variant="fullWidth"
            centered
          >
            <Tab label="Test Results" />
            <Tab
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                  Triage
                  <Tooltip
                    title="Tests are grouped by status and failure reason, statuses are ordered by severity."
                    arrow
                    placement="top"
                  >
                    <IconButton size="small" sx={{ p: 0 }}>
                      <HelpOutlineIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </Box>
              }
            />
          </Tabs>
        </Box>
        <Box sx={{ flexGrow: 1, minHeight: 0, overflow: 'hidden' }}>
          {activeTab === 0 && (
            <AggregationView
              ref={activeViewRef}
              autoLocate={props.autoLocate}
              {...props}
            />
          )}
          {activeTab === 1 && (
            <TriageView
              ref={activeViewRef}
              autoLocate={props.autoLocate}
              {...props}
            />
          )}
        </Box>
      </Box>
    </TestAggregationProvider>
  );
}
