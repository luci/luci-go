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
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

import { TreeStatusSideNav } from './tree_status_side_nav';

interface AlertsSideNavProps {
  tree: TreeJson;
  selectedTab: string | null;
  setSelectedTab: (tab: string) => void;
  topLevelAlerts: StructuredAlert[];
  ungroupedTopLevelAlerts: StructuredAlert[];
  alertGroups: readonly AlertGroup[];
}

export const AlertsSideNav = ({
  tree,
  selectedTab,
  setSelectedTab,
  topLevelAlerts,
  ungroupedTopLevelAlerts,
  alertGroups,
}: AlertsSideNavProps) => {
  return (
    <>
      <List>
        <TreeStatusSideNav tree={tree} />
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
        <ListItem disablePadding key={group.name}>
          <ListItemButton
            selected={selectedTab === 'group:' + group.name}
            onClick={() => setSelectedTab('group:' + group.name)}
          >
            <ListItemIcon>
              <FolderIcon />
            </ListItemIcon>
            <ListItemText primary={group.displayName} />
            <Chip label={group.alertKeys.length} variant="outlined" />
          </ListItemButton>
        </ListItem>
      ))}
    </>
  );
};
