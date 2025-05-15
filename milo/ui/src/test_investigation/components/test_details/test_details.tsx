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

import { Box, Tab, Tabs, Typography } from '@mui/material';
import { JSX, useState } from 'react';

import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';
import { TestVariant } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_variant.pb';

import { TestTabContent } from './test_tab_content'; // Correct path

interface TestDetailsProps {
  invocation: Invocation;
  testVariant: TestVariant;
}

export function TestDetails({
  invocation,
  testVariant,
}: TestDetailsProps): JSX.Element {
  const [selectedTab, setSelectedTab] = useState(0);

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) =>
    setSelectedTab(newValue);

  const placeholderInvocationDetails = `Invocation Details Placeholder (Realm: ${invocation.realm})`;
  const placeholderPerformance = 'Performance Content Placeholder - TODO';
  const placeholderChanges = 'Changes Content Placeholder - TODO';
  const placeholderFeatureFlags = 'Feature Flags Content Placeholder - TODO';

  return (
    <>
      <Tabs
        value={selectedTab}
        onChange={handleTabChange}
        aria-label="Test Details Sections"
      >
        <Tab label="Test" />
        <Tab label="Invocation details" />
        <Tab label="Performance" />
        <Tab label="Changes" />
        <Tab label="Feature flags" />
      </Tabs>

      <Box sx={{ paddingTop: '24px' }}>
        {selectedTab === 0 && (
          <TestTabContent // This now handles content for the "Test" tab
            invocation={invocation}
            testVariant={testVariant}
          />
        )}
        {selectedTab === 1 && (
          <Typography>{placeholderInvocationDetails}</Typography>
        )}
        {selectedTab === 2 && <Typography>{placeholderPerformance}</Typography>}
        {selectedTab === 3 && <Typography>{placeholderChanges}</Typography>}
        {selectedTab === 4 && (
          <Typography>{placeholderFeatureFlags}</Typography>
        )}
      </Box>
    </>
  );
}
