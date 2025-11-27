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

import SearchIcon from '@mui/icons-material/Search';
import TuneIcon from '@mui/icons-material/Tune';
import {
  Box,
  Chip,
  ClickAwayListener,
  IconButton,
  InputAdornment,
  TextField,
  Typography,
} from '@mui/material';
import { useMemo, useRef } from 'react';

import { ClusteringControls } from '../clustering_controls';
import { useArtifactsContext } from '../context';

import { ArtifactFiltersDropdown } from './artifact_filters_dropdown';
import { useArtifactFilters } from './context/';

export interface ArtifactsTreeLayoutProps {
  children: React.ReactNode;
}

export function ArtifactsTreeLayout({ children }: ArtifactsTreeLayoutProps) {
  const { clusteredFailures, hasRenderableResults, selectedArtifact } =
    useArtifactsContext();
  const { searchTerm, setSearchTerm, isFilterPanelOpen, setIsFilterPanelOpen } =
    useArtifactFilters();

  const filterContainerRef = useRef<HTMLDivElement>(null);

  const noFailuresToClusterMessage = 'No results to cluster.';

  const selectedArtifactLabel = useMemo(() => {
    if (selectedArtifact) {
      if (selectedArtifact.isSummary) {
        return 'Summary';
      } else if (selectedArtifact.artifact) {
        return selectedArtifact.artifact.artifactId;
      }
    }
    return '';
  }, [selectedArtifact]);

  return (
    <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <Box
        sx={{ p: 1, pb: 0, display: 'flex', flexDirection: 'column', gap: 2 }}
      >
        {clusteredFailures.length > 0 ? (
          <ClusteringControls />
        ) : (
          hasRenderableResults && (
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              {noFailuresToClusterMessage}
            </Typography>
          )
        )}
        <ClickAwayListener onClickAway={() => setIsFilterPanelOpen(false)}>
          <Box ref={filterContainerRef} sx={{ position: 'relative' }}>
            <TextField
              placeholder="Search for artifact"
              variant="outlined"
              size="small"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              fullWidth
              slotProps={{
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <SearchIcon />
                    </InputAdornment>
                  ),
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        onClick={() => setIsFilterPanelOpen((prev) => !prev)}
                        sx={{
                          color: isFilterPanelOpen ? 'primary.main' : 'inherit',
                        }}
                      >
                        <TuneIcon />
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
              sx={{
                '& .MuiOutlinedInput-root': {
                  borderRadius: '50px',
                  backgroundColor: 'action.hover',
                  '& fieldset': {
                    border: 'none',
                  },
                },
              }}
            />
            {isFilterPanelOpen && (
              <Box
                sx={{
                  position: 'absolute',
                  top: 'calc(100% + 4px)',
                  left: 0,
                  width: '100%',
                  zIndex: (theme) => theme.zIndex.tooltip,
                }}
              >
                <ArtifactFiltersDropdown />
              </Box>
            )}
          </Box>
        </ClickAwayListener>
        {selectedArtifact && (
          <Box>
            Selected artifact:{' '}
            <Chip size="small" label={selectedArtifactLabel} sx={{ ml: 1 }} />
          </Box>
        )}
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          width: '100%',
          wordBreak: 'break-word',
          overflowY: 'auto',
        }}
      >
        {children}
      </Box>
    </Box>
  );
}
