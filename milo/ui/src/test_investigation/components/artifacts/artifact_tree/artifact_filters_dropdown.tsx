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
  Box,
  Button,
  FormControl,
  FormControlLabel,
  MenuItem,
  Paper,
  Select,
  Switch,
  Typography,
  SelectChangeEvent,
} from '@mui/material';
import { ChangeEvent } from 'react';

import { useArtifactFilters } from './context';

export function ArtifactFiltersDropdown() {
  const {
    availableArtifactTypes,
    artifactTypes,
    setArtifactTypes,
    crashTypes,
    setCrashTypes,
    showCriticalCrashes,
    setShowCriticalCrashes,
    hideAutomationFiles,
    setHideAutomationFiles,
    hideEmptyFolders,
    setHideEmptyFolders,
    showOnlyFoldersWithError,
    setShowOnlyFoldersWithError,
    onClearFilters,
    setIsFilterPanelOpen,
  } = useArtifactFilters();

  return (
    <Paper
      elevation={3}
      sx={{ p: 2, borderTop: '1px solid', borderColor: 'divider' }}
    >
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            mb: 1,
          }}
        >
          <Typography variant="subtitle2" sx={{ fontWeight: 'medium' }}>
            Filter artifacts
          </Typography>
          <Box>
            <Button onClick={onClearFilters} size="small">
              Clear
            </Button>
            <Button
              onClick={() => setIsFilterPanelOpen(false)}
              size="small"
              sx={{ ml: 1 }}
            >
              Close
            </Button>
          </Box>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <Typography variant="body2" sx={{ minWidth: '100px' }}>
            Artifact types
          </Typography>
          <FormControl fullWidth size="small">
            <Select
              multiple
              value={artifactTypes}
              onChange={(e: SelectChangeEvent<string[]>) =>
                setArtifactTypes(e.target.value as string[])
              }
              renderValue={(selected) => selected.join(', ')}
              displayEmpty
              MenuProps={{ disablePortal: true }}
            >
              <MenuItem value="" disabled>
                <Typography variant="body2" color="text.secondary">
                  Select types
                </Typography>
              </MenuItem>
              {availableArtifactTypes.map((type) => (
                <MenuItem key={type} value={type}>
                  {type}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <Typography variant="body2" sx={{ minWidth: '100px' }}>
            Crash types
          </Typography>
          <FormControl fullWidth size="small">
            <Select
              multiple
              value={crashTypes}
              onChange={(e: SelectChangeEvent<string[]>) =>
                setCrashTypes(e.target.value as string[])
              }
              renderValue={(selected) => selected.join(', ')}
              displayEmpty
              disabled
              MenuProps={{ disablePortal: true }}
            >
              <MenuItem value="" disabled>
                <Typography variant="body2" color="text.secondary">
                  Select types
                </Typography>
              </MenuItem>
            </Select>
          </FormControl>
        </Box>

        <FormControlLabel
          control={
            <Switch
              checked={showCriticalCrashes}
              onChange={(e: ChangeEvent<HTMLInputElement>) =>
                setShowCriticalCrashes(e.target.checked)
              }
              disabled
            />
          }
          label="Show only critical crashes"
          sx={{ ml: 0, justifyContent: 'space-between', mr: 0 }}
          labelPlacement="start"
        />
        <FormControlLabel
          control={
            <Switch
              checked={hideAutomationFiles}
              onChange={(e: ChangeEvent<HTMLInputElement>) =>
                setHideAutomationFiles(e.target.checked)
              }
              disabled
            />
          }
          label="Hide (##) files for automation"
          sx={{ ml: 0, justifyContent: 'space-between', mr: 0 }}
          labelPlacement="start"
        />
        <FormControlLabel
          control={
            <Switch
              checked={hideEmptyFolders}
              onChange={(e: ChangeEvent<HTMLInputElement>) =>
                setHideEmptyFolders(e.target.checked)
              }
            />
          }
          label="Hide empty folders"
          sx={{ ml: 0, justifyContent: 'space-between', mr: 0 }}
          labelPlacement="start"
        />
        <FormControlLabel
          control={
            <Switch
              checked={showOnlyFoldersWithError}
              onChange={(e: ChangeEvent<HTMLInputElement>) =>
                setShowOnlyFoldersWithError(e.target.checked)
              }
              disabled
            />
          }
          label="Show only folders with error"
          sx={{ ml: 0, justifyContent: 'space-between', mr: 0 }}
          labelPlacement="start"
        />
      </Box>
    </Paper>
  );
}
