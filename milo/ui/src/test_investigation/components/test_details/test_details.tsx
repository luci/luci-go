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

import { Box, Tab, Tabs } from '@mui/material';
import { useState } from 'react';

import { TestTabContent } from './test_tab_content'; // Correct path

export function TestDetails() {
  const [selectedTab, setSelectedTab] = useState(0);

  const handleTabChange = (_event: React.SyntheticEvent, newValue: number) =>
    setSelectedTab(newValue);

  return (
    <>
      <Tabs
        value={selectedTab}
        onChange={handleTabChange}
        aria-label="Test Details Sections"
      >
        <Tab label="Test" />
      </Tabs>

      <Box>{selectedTab === 0 && <TestTabContent />}</Box>
    </>
  );
}
