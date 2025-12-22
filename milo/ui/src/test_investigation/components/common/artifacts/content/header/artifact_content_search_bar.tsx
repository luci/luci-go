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

import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import SearchIcon from '@mui/icons-material/Search';
import {
  Divider as MuiDivider,
  IconButton,
  InputAdornment,
  TextField,
  Typography,
} from '@mui/material';
import { ChangeEvent } from 'react';

interface ArtifactContentSearchBarProps {
  searchQuery: string;
  onSearchChange: (e: ChangeEvent<HTMLInputElement>) => void;
  currentMatchIndex: number;
  totalMatches: number;
  onNext: () => void;
  onPrev: () => void;
}

export function ArtifactContentSearchBar({
  searchQuery,
  onSearchChange,
  currentMatchIndex,
  totalMatches,
  onNext,
  onPrev,
}: ArtifactContentSearchBarProps) {
  return (
    <TextField
      size="small"
      placeholder="Search query"
      value={searchQuery}
      onChange={onSearchChange}
      InputProps={{
        startAdornment: (
          <InputAdornment position="start">
            <SearchIcon fontSize="small" />
          </InputAdornment>
        ),
        endAdornment: (
          <InputAdornment position="end">
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{
                mr: 1,
                minWidth: 40,
                textAlign: 'right',
                userSelect: 'none',
              }}
            >
              {totalMatches > 0
                ? `${currentMatchIndex + 1}/${totalMatches}`
                : '0/0'}
            </Typography>
            <MuiDivider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
            <IconButton
              size="small"
              onClick={onNext}
              disabled={totalMatches === 0}
              title="Next match"
              sx={{ p: 0.5 }}
            >
              <ExpandMoreIcon fontSize="small" />
            </IconButton>
            <IconButton
              size="small"
              onClick={onPrev}
              disabled={totalMatches === 0}
              title="Previous match"
              sx={{ p: 0.5 }}
            >
              <ExpandLessIcon fontSize="small" />
            </IconButton>
          </InputAdornment>
        ),
      }}
      sx={{
        width: '100%',
        '& .MuiOutlinedInput-root': {
          paddingRight: 1,
        },
      }}
    />
  );
}
