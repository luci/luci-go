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

import BugReportIcon from '@mui/icons-material/BugReport';
import FlakyIcon from '@mui/icons-material/Flaky';
import FolderIcon from '@mui/icons-material/Folder';
import InboxIcon from '@mui/icons-material/Inbox';
import {
  Box,
  Button,
  Chip,
  Divider,
  Link,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  ListSubheader,
} from '@mui/material';
import { useEffect, useState } from 'react';

import { TreeJson } from '@/monitoring/util/server_json';
import { AlertOrganizer, StructuredAlert } from '@/monitoringv2/util/alerts';
import { AlertGroup } from '@/proto/go.chromium.org/luci/luci_notify/api/service/v1/alert_groups.pb';

import { TreeStatusSideNav } from './tree_status_side_nav';

interface AlertsSideNavProps {
  tree: TreeJson;
  selectedTab: string | null;
  setSelectedTab: (tab: string) => void;
  topLevelAlerts: StructuredAlert[];
  ungroupedTopLevelAlerts: StructuredAlert[];
  alertGroups: readonly AlertGroup[];
  organizer: AlertOrganizer;
}

export const AlertsSideNav = ({
  tree,
  selectedTab,
  setSelectedTab,
  topLevelAlerts,
  ungroupedTopLevelAlerts,
  alertGroups,
  organizer,
}: AlertsSideNavProps) => {
  const [defaultShowResolved, setDefaultShowResolved] = useState(false);
  const [userShowResolved, setUserShowResolved] = useState<boolean | undefined>(
    undefined,
  );
  const activeGroupKeys = organizer.activeAlertKeys();
  const numResolvedGroups = alertGroups.filter(
    (group) => activeGroupKeys[group.name].length === 0,
  ).length;

  // We want to show the resolved groups on the side if the current group is resolved
  // else we want to hide them for reduced clutter.
  // We also allow the user to override this (with userShowResolved).
  useEffect(() => {
    if (selectedTab?.startsWith('group:')) {
      setDefaultShowResolved(
        activeGroupKeys[selectedTab.slice(6)]?.length === 0,
      );
    }
  }, [selectedTab, activeGroupKeys]);
  const showResolved = userShowResolved ?? defaultShowResolved;
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
      {alertGroups
        .filter((group) => activeGroupKeys[group.name].length > 0)
        .map((group) => (
          <ListItem disablePadding key={group.name}>
            <ListItemButton
              selected={selectedTab === 'group:' + group.name}
              onClick={() => setSelectedTab('group:' + group.name)}
            >
              <ListItemIcon>
                <FolderIcon />
              </ListItemIcon>
              <ListItemText primary={group.displayName} />
              <Chip
                label={activeGroupKeys[group.name].length}
                variant="outlined"
              />
            </ListItemButton>
          </ListItem>
        ))}
      {alertGroups.length - numResolvedGroups === 0 && (
        <ListItem>
          <Box sx={{ opacity: '50%' }}>No active alert groups</Box>
        </ListItem>
      )}
      {showResolved && numResolvedGroups > 0 && (
        <>
          <Divider />
          <ListSubheader component="div" id="nested-list-subheader">
            Resolved Groups
          </ListSubheader>
          {alertGroups
            .filter((group) => activeGroupKeys[group.name].length === 0)
            .map((group) => (
              <ListItem disablePadding key={group.name}>
                <ListItemButton
                  selected={selectedTab === 'group:' + group.name}
                  onClick={() => setSelectedTab('group:' + group.name)}
                >
                  <ListItemIcon>
                    <FolderIcon />
                  </ListItemIcon>
                  <ListItemText primary={group.displayName} />
                  <Chip
                    label={activeGroupKeys[group.name].length}
                    variant="outlined"
                  />
                </ListItemButton>
              </ListItem>
            ))}
        </>
      )}
      {numResolvedGroups > 0 && (
        <ListItem>
          <Button
            onClick={() => setUserShowResolved(!showResolved)}
            variant="outlined"
            color="inherit"
            sx={{ opacity: '50%' }}
            size="small"
          >
            {!showResolved ? (
              <>
                Show{' '}
                {
                  alertGroups.filter(
                    (group) => activeGroupKeys[group.name].length === 0,
                  ).length
                }{' '}
                resolved groups
              </>
            ) : (
              'Hide resolved groups'
            )}
          </Button>
        </ListItem>
      )}

      <Divider />
      <ListSubheader component="div" id="nested-list-subheader">
        Next Steps
      </ListSubheader>
      {tree.hotlistId && (
        <ListItem disablePadding>
          <ListItemButton
            component={Link}
            href={`https://b.corp.google.com/issues?q=hotlistid:${tree.hotlistId}%20status:open`}
            target="_blank"
          >
            <ListItemIcon>
              <BugReportIcon />
            </ListItemIcon>
            <ListItemText primary="Review Monitoring Bugs" />
          </ListItemButton>
        </ListItem>
      )}
      {tree.project && (
        <ListItem disablePadding>
          <ListItemButton
            component={Link}
            href={
              `/ui/tests/p/${tree.project}/clusters` +
              '?interval=7d&selectedMetrics=test-runs-failed%2Cbuilds-failed-due-to-flaky-tests&orderBy=builds-failed-due-to-flaky-tests'
            }
            target="_blank"
          >
            <ListItemIcon>
              <FlakyIcon />
            </ListItemIcon>
            <ListItemText primary="Investigate Top Flaky Tests" />
          </ListItemButton>
        </ListItem>
      )}
    </>
  );
};
