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

import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import {
  Box,
  Button,
  CircularProgress,
  ToggleButton,
  ToggleButtonGroup,
  Menu,
  Tooltip,
  Typography,
  MenuItem,
  ListItemIcon,
  Checkbox,
  Divider,
} from '@mui/material';
import { useState } from 'react';
import { useUpdateEffect } from 'react-use';

import { SearchInput } from '@/common/components/search_input';
import { useAlertGroups } from '@/monitoringv2/hooks/alert_groups';
import {
  useAlerts,
  useTree,
} from '@/monitoringv2/pages/monitoring_page/context';
import {
  AlertKind,
  AlertOrganizer,
  filterAlerts,
  StructuredAlert,
} from '@/monitoringv2/util/alerts';

import { AlertTable } from '../alert_table';

import { AlertsSideNav } from './alerts_side_nav';
import { AllHeader } from './headers/all_header';
import { GroupHeader } from './headers/group_header';
import { UngroupedHeader } from './headers/ungrouped_header';
import { DEFAULT_ALERT_TAB, useFilterQuery, useSelectedTab } from './hooks';

export const Alerts = () => {
  const tree = useTree();
  const [filter, setFilter] = useFilterQuery('');
  const {
    builderAlerts,
    stepAlerts,
    testAlerts,
    alertsLoading,
    alertsLoadingStatus,
  } = useAlerts();
  const [organizeBy, setOrganizeBy] = useState<AlertKind>('builder');
  const [showOptions, setShowOptions] = useState<string[]>([]);
  const [selectedTab, setSelectedTab] = useSelectedTab(DEFAULT_ALERT_TAB);

  useUpdateEffect(() => {
    setFilter('');
    // Although we use setFilter here, we don't want to run this every time setFilter changes
    // because otherwise we cannot actually enter a filter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedTab]);

  const alertGroupsData = useAlertGroups();

  if (!tree) {
    return <></>;
  }

  const alertGroups = alertGroupsData?.alertGroups || [];
  const groupedAlertKeys = Object.fromEntries(
    alertGroups.map((g) => [
      g.name,
      g.alertKeys.map((k) => decodeURIComponent(k.slice('alerts/'.length))),
    ]),
  );
  const organizer = new AlertOrganizer(
    [...builderAlerts, ...stepAlerts, ...testAlerts],
    groupedAlertKeys,
    {
      showResolved: showOptions.includes('resolved'),
      showFlaky: showOptions.includes('flaky'),
      showChildrenHidden: showOptions.includes('children_hidden'),
    },
  );

  const allAlerts = organizer.allAlerts(organizeBy);
  const ungroupedAlerts = organizer.ungroupedAlerts(organizeBy);
  const groupedAlerts = Object.fromEntries(
    alertGroups.map((g) => [g.name, organizer.groupAlerts(g.name)]),
  );
  const numHiddenAlerts = organizer.numHiddenAlerts(organizeBy);

  let selectedGroup = undefined;
  let header = null;
  let selectedAlerts: StructuredAlert[] = [];
  if (selectedTab === 'ungrouped') {
    selectedAlerts = ungroupedAlerts;
    header = <UngroupedHeader />;
  } else if (selectedTab === 'all') {
    selectedAlerts = allAlerts;
    header = <AllHeader />;
  } else if (selectedTab.startsWith('group:')) {
    const groupName = selectedTab.slice(6);
    const groupIndex = alertGroups.findIndex((g) => g.name === groupName);
    const group = alertGroups[groupIndex];
    if (group) {
      selectedGroup = group;
      selectedAlerts = groupedAlerts[groupName];
      header = <GroupHeader group={group} />;
    }
  }

  const filteredAlerts = filterAlerts(selectedAlerts, filter);
  return (
    <Box sx={{ display: 'flex' }}>
      <Box
        sx={{
          minWidth: '256px',
          maxWidth: '25%',
          flexGrow: 1,
          flexShrink: 0,
        }}
      >
        <AlertsSideNav
          tree={tree}
          selectedTab={selectedTab}
          setSelectedTab={setSelectedTab}
          topLevelAlerts={allAlerts}
          ungroupedTopLevelAlerts={ungroupedAlerts}
          alertGroups={alertGroups}
        />
      </Box>
      <Box sx={{ flexGrow: 3 }}>
        <Box
          sx={{
            position: 'sticky',
            top: 'var(--accumulated-top)',
            zIndex: 30,
            backgroundColor: '#fff',
            padding: '8px 16px',
          }}
        >
          <Box sx={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
            <ToggleButtonGroup
              id="organizeByGroup"
              exclusive
              size="small"
              value={organizeBy}
              onChange={(_, v) => setOrganizeBy(v)}
            >
              <ToggleButton value="builder">Builder</ToggleButton>
              <ToggleButton value="step">Step</ToggleButton>
              <ToggleButton value="test">Test</ToggleButton>
            </ToggleButtonGroup>
            <FilterAlertInput />
            <Tooltip title={'Select alerts to show'}>
              <ShowOptionsButton
                showOptions={showOptions}
                setShowOptions={setShowOptions}
              />
            </Tooltip>
          </Box>
        </Box>
        {header ? header : null}
        <Box
          sx={{
            height: '20px',
            padding: '0 10px',
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
          }}
        >
          {alertsLoading ? (
            <>
              <CircularProgress size={20} />
              <Typography variant="body2">{alertsLoadingStatus}</Typography>
            </>
          ) : null}
        </Box>
        <AlertTable
          alerts={filteredAlerts}
          group={selectedGroup}
          groups={alertGroups}
          selectedTab={selectedTab}
        />
        {numHiddenAlerts > 0 && (
          <Box sx={{ mt: '16px', textAlign: 'center' }}>
            {numHiddenAlerts} alert{numHiddenAlerts > 1 ? 's' : ''} hidden for
            suspected flakiness.{' '}
            <Button
              onClick={() =>
                setShowOptions([...showOptions, 'children_hidden'])
              }
            >
              Show all
            </Button>
          </Box>
        )}
      </Box>
    </Box>
  );
};

function ShowOptionsButton({
  showOptions,
  setShowOptions,
}: {
  showOptions: string[];
  setShowOptions: (options: string[]) => void;
}) {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);
  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget);
  };
  const handleClose = () => {
    setAnchorEl(null);
  };
  const toggleOption = (option: string) => {
    if (showOptions.includes(option)) {
      setShowOptions(showOptions.filter((o) => o !== option));
    } else {
      setShowOptions([...showOptions, option]);
    }
  };
  return (
    <>
      <Button
        variant="outlined"
        color="inherit"
        onClick={handleClick}
        endIcon={<KeyboardArrowDownIcon />}
      >
        Show
      </Button>
      <Menu
        id="show-options-menu"
        anchorEl={anchorEl}
        open={open}
        onClose={handleClose}
      >
        <MenuItem onClick={() => setShowOptions([])}>Reset to default</MenuItem>
        <Divider />
        <CheckedMenuItem
          onCheck={() => toggleOption('children_hidden')}
          checked={showOptions.includes('children_hidden')}
          text="Alerts with all children hidden"
        />
        <CheckedMenuItem
          onCheck={() => toggleOption('flaky')}
          checked={showOptions.includes('flaky')}
          text="Single occurrence alerts"
        />
        <CheckedMenuItem
          onCheck={() => toggleOption('resolved')}
          checked={showOptions.includes('resolved')}
          text="Resolved alerts"
        />
      </Menu>
    </>
  );
}

function CheckedMenuItem({
  checked,
  onCheck,
  text,
}: {
  checked: boolean;
  onCheck: () => void;
  text: string;
}) {
  return (
    <MenuItem onClick={onCheck}>
      <ListItemIcon>
        <Checkbox checked={checked} sx={{ paddingLeft: 0 }} />
      </ListItemIcon>
      {text}
    </MenuItem>
  );
}

function FilterAlertInput() {
  const [filter, updateFilter] = useFilterQuery('');
  return (
    <SearchInput
      initDelayMs={200}
      onValueChange={(value) => updateFilter(value)}
      size="small"
      value={filter}
      placeholder="Filter Alerts"
    />
  );
}
