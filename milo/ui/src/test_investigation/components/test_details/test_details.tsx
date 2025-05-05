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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { useState } from 'react';

export function TestDetails() {
  const [selectedTab, setSelectedTab] = useState(0);

  const handleTabChange = (_: React.SyntheticEvent, newValue: number) => {
    setSelectedTab(newValue);
  };

  const tagsPanelId = 'tags-content';
  const tagsHeaderId = 'tags-header';
  const artifactsPanelId = 'artifacts-content';
  const artifactsHeaderId = 'artifacts-header';
  return (
    <>
      <Tabs
        value={selectedTab}
        onChange={handleTabChange}
        aria-label="Page Sections"
      >
        <Tab label="Test" />
        <Tab label="Invocation artifacts" />
        <Tab label="Invocation details" />
        <Tab label="Performance" />
        <Tab label="Changes" />
        <Tab label="Feature flags" />
      </Tabs>

      <Box sx={{ paddingTop: '24px' }}>
        {selectedTab === 0 && (
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
            <Typography sx={{ mb: 1 }}>
              Test Content Placeholder Area
            </Typography>

            <Accordion>
              <AccordionSummary
                expandIcon={<ExpandMoreIcon />}
                aria-controls={tagsPanelId}
                id={tagsHeaderId}
              >
                <Typography
                  sx={{
                    fontWeight: 400,
                    fontSize: '20px',
                    color: 'var(--gm3-color-on-surface-strong)',
                  }}
                >
                  Tags
                </Typography>
              </AccordionSummary>
              <AccordionDetails id={tagsPanelId}>
                <Typography>Tag content goes here... (Placeholder)</Typography>
              </AccordionDetails>
            </Accordion>

            <Accordion defaultExpanded>
              <AccordionSummary
                expandIcon={<ExpandMoreIcon />}
                aria-controls={artifactsPanelId}
                id={artifactsHeaderId}
              >
                <Typography
                  sx={{
                    fontWeight: 400,
                    fontSize: '20px',
                    color: 'var(--gm3-color-on-surface-strong)',
                  }}
                >
                  Artifacts
                </Typography>
              </AccordionSummary>
              <AccordionDetails id={artifactsPanelId}>
                <Typography>
                  Artifacts content (like the file tree/viewer) goes here...
                  (Placeholder)
                </Typography>
              </AccordionDetails>
            </Accordion>
          </Box>
        )}

        {selectedTab === 1 && (
          <Typography>Invocation Artifacts Content Placeholder</Typography>
        )}
        {selectedTab === 2 && (
          <Typography>Invocation Details Content Placeholder</Typography>
        )}
        {selectedTab === 3 && (
          <Typography>Performance Content Placeholder</Typography>
        )}
        {selectedTab === 4 && (
          <Typography>Changes Content Placeholder</Typography>
        )}
        {selectedTab === 5 && (
          <Typography>Feature Flags Content Placeholder</Typography>
        )}
      </Box>
    </>
  );
}
