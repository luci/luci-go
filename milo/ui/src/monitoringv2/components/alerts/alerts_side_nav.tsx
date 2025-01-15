// Copyright 2024 The LUCI Authors.
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

import FolderIcon from '@mui/icons-material/Folder';
import InboxIcon from '@mui/icons-material/Inbox';
import ParkIcon from '@mui/icons-material/Park';
import {
  Chip,
  Divider,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  ListSubheader,
} from '@mui/material';

import { TreeJson } from '@/monitoring/util/server_json';
import { StructuredAlert } from '@/monitoringv2/util/alerts';

import { AlertGroup } from './alerts';

interface AlertsSideNavProps {
  tree: TreeJson;
  selectedTab: string | null;
  setSelectedTab: (tab: string) => void;
  topLevelAlerts: StructuredAlert[];
  ungroupedTopLevelAlerts: StructuredAlert[];
  alertGroups: AlertGroup[];
}

export const AlertsSideNav = ({
  selectedTab,
  setSelectedTab,
  topLevelAlerts,
  ungroupedTopLevelAlerts,
  alertGroups,
}: AlertsSideNavProps) => {
  return (
    <>
      <List>
        <ListItem disablePadding>
          <ListItemButton>
            <ListItemIcon>
              <ParkIcon sx={{ color: 'var(--success-color)' }} />
            </ListItemIcon>
            <ListItemText
              primary="Tree Status"
              sx={{ color: 'var(--success-color)' }}
            />
            <Chip
              label="Open"
              variant="outlined"
              sx={{
                color: 'var(--success-color)',
                borderColor: 'var(--success-color)',
              }}
            />
          </ListItemButton>
        </ListItem>
        <Divider />
        <ListSubheader component="div" id="nested-list-subheader">
          Alerts
        </ListSubheader>
        <ListItem disablePadding>
          <ListItemButton
            selected={selectedTab === 'ungrouped'}
            onClick={() => setSelectedTab('ungrouped')}
          >
            <ListItemIcon>
              <InboxIcon />
            </ListItemIcon>
            <ListItemText primary="Ungrouped" />
            {/* FIXME: This chip logic needs to be updated! */}
            <Chip
              label={ungroupedTopLevelAlerts.length}
              variant={
                ungroupedTopLevelAlerts.length > 1 ? 'filled' : 'outlined'
              }
              color={ungroupedTopLevelAlerts.length > 1 ? 'primary' : undefined}
            />
          </ListItemButton>
        </ListItem>
        <ListItem disablePadding>
          <ListItemButton
            selected={selectedTab === 'all'}
            onClick={() => setSelectedTab('all')}
          >
            <ListItemIcon>
              <InboxIcon />
            </ListItemIcon>
            <ListItemText primary="All" />
            <Chip label={topLevelAlerts.length} variant="outlined" />
          </ListItemButton>
        </ListItem>
      </List>
      <Divider />
      <ListSubheader component="div" id="nested-list-subheader">
        Groups
      </ListSubheader>
      {alertGroups.map((group) => (
        <ListItem disablePadding key={group.id}>
          <ListItemButton
            selected={selectedTab === 'group:' + group.id}
            onClick={() => setSelectedTab('group:' + group.id)}
          >
            <ListItemIcon>
              <FolderIcon />
            </ListItemIcon>
            <ListItemText primary={group.name} />
            <Chip label={group.alertKeys.length} variant="outlined" />
          </ListItemButton>
        </ListItem>
      ))}
    </>
  );
};
