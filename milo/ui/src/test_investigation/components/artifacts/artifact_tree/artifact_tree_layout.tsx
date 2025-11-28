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

import {
  AccountTree as AccountTreeIcon,
  Description as DescriptionIcon,
  HelpOutline,
} from '@mui/icons-material';
import {
  Box,
  Divider,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from '@mui/material';
import { ReactNode } from 'react';

import { ClusteringControls } from '../clustering_controls';
import { useArtifactsContext } from '../context';

import { ArtifactFiltersDropdown } from './artifact_filters_dropdown';
import { useArtifactFilters } from './context/context';

interface ArtifactsTreeLayoutProps {
  children: ReactNode;
  viewMode: 'artifacts' | 'work-units';
  onViewModeChange: (mode: 'artifacts' | 'work-units') => void;
}

export function ArtifactsTreeLayout({
  children,
  viewMode,
  onViewModeChange,
}: ArtifactsTreeLayoutProps) {
  const { clusteredFailures, hasRenderableResults } = useArtifactsContext();

  const { isFilterPanelOpen, setIsFilterPanelOpen } = useArtifactFilters();

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        width: '100%',
        overflow: 'hidden',
      }}
    >
      <Box sx={{ flexShrink: 0, p: 1 }}>
        {hasRenderableResults && clusteredFailures && (
          <Box sx={{ mb: 1 }}>
            <ClusteringControls />
          </Box>
        )}

        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 0.5,
              flexShrink: 0,
            }}
          >
            <Typography variant="caption" color="text.secondary">
              View Mode
            </Typography>
            <Tooltip title="Switch between viewing artifacts as a directory structure or grouped by work units">
              <HelpOutline
                sx={{ fontSize: 14, color: 'text.secondary', cursor: 'help' }}
              />
            </Tooltip>
          </Box>
          <ToggleButtonGroup
            value={viewMode}
            exclusive
            onChange={(_, newMode) => {
              if (newMode) onViewModeChange(newMode);
            }}
            size="small"
            fullWidth
            sx={{ flex: 1 }}
            aria-label="artifact view mode"
          >
            <ToggleButton value="artifacts" aria-label="artifacts view">
              <Tooltip title="View raw artifacts as a directory structure">
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <DescriptionIcon fontSize="small" />
                  Directory
                </Box>
              </Tooltip>
            </ToggleButton>
            <ToggleButton value="work-units" aria-label="work units view">
              <Tooltip title="View artifacts grouped by work units">
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <AccountTreeIcon fontSize="small" />
                  Work Units
                </Box>
              </Tooltip>
            </ToggleButton>
          </ToggleButtonGroup>
        </Box>

        <ArtifactFiltersDropdown
          isOpen={isFilterPanelOpen}
          onToggle={() => setIsFilterPanelOpen((prev) => !prev)}
        />
      </Box>

      <Divider />

      <Box sx={{ flexGrow: 1, overflow: 'hidden', position: 'relative' }}>
        {children}
      </Box>
    </Box>
  );
}
