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

import AdbIcon from '@mui/icons-material/Adb';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import {
  IconButton,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Switch,
} from '@mui/material';
import { useState, MouseEvent } from 'react';

import {
  getAndroidBugToolLink,
  getRawArtifactURLPath,
} from '@/common/tools/url_utils';
import { Artifact } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { Invocation } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/invocation.pb';

interface ArtifactContentSearchMenuProps {
  artifact: Artifact;
  isLogComparisonPossible: boolean;
  showLogComparison: boolean;
  hasPassingResults: boolean;
  onToggleLogComparison: () => void;
  isAnTS: boolean;
  invocation: Invocation;
  actions?: React.ReactNode;
}

export function ArtifactContentSearchMenu({
  artifact,
  isLogComparisonPossible,
  showLogComparison,
  hasPassingResults,
  onToggleLogComparison,
  isAnTS,
  invocation,
  actions,
}: ArtifactContentSearchMenuProps) {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const openMenu = Boolean(anchorEl);

  const handleMenuClick = (event: MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  return (
    <>
      {actions}
      <IconButton
        size="small"
        onClick={handleMenuClick}
        aria-controls={openMenu ? 'artifact-menu' : undefined}
        aria-haspopup="true"
        aria-expanded={openMenu ? 'true' : undefined}
        aria-label="Options"
      >
        <MoreVertIcon />
      </IconButton>
      <Menu
        anchorEl={anchorEl}
        id="artifact-menu"
        open={openMenu}
        onClose={handleMenuClose}
        onClick={handleMenuClose}
        transformOrigin={{ horizontal: 'right', vertical: 'top' }}
        anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
      >
        {isLogComparisonPossible && (
          <MenuItem
            disabled={!hasPassingResults}
            onClick={() => onToggleLogComparison()}
          >
            <ListItemIcon>{/* No icon or custom icon */}</ListItemIcon>
            <ListItemText primary="Log Comparison" />
            <Switch
              edge="end"
              size="small"
              checked={showLogComparison}
              inputProps={{
                'aria-labelledby': 'switch-list-label-log-comparison',
              }}
              onClick={(e) => {
                e.stopPropagation();
                onToggleLogComparison();
              }}
            />
          </MenuItem>
        )}
        {artifact.fetchUrl && (
          <MenuItem
            component="a"
            href={getRawArtifactURLPath(artifact.name)}
            target="_blank"
            rel="noopener noreferrer"
          >
            <ListItemIcon>
              <OpenInNewIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText>Open Raw</ListItemText>
          </MenuItem>
        )}
        {isAnTS && (
          <MenuItem
            component="a"
            href={getAndroidBugToolLink(artifact.artifactId, invocation)}
            target="_blank"
            rel="noopener noreferrer"
          >
            <ListItemIcon>
              <AdbIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText>Open in ABT</ListItemText>
          </MenuItem>
        )}
      </Menu>
    </>
  );
}
