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

import VisibilityIcon from '@mui/icons-material/Visibility';
import {
  Box,
  Button,
  CircularProgress,
  ToggleButton,
  ToggleButtonGroup,
  Tooltip,
  Typography,
} from '@mui/material';
import { useState } from 'react';

import { SearchInput } from '@/common/components/search_input';
import {
  useAlerts,
  useTree,
} from '@/monitoringv2/pages/monitoring_page/context';
import {
  AlertKind,
  buildStructuredAlertsPreSorted,
  filterAlerts,
  GenericAlert,
} from '@/monitoringv2/util/alerts';

import { AlertTable } from '../alert_table';

import { AlertsSideNav } from './alerts_side_nav';
import { AllHeader } from './headers/all_header';
import { GroupHeader } from './headers/group_header';
import { UngroupedHeader } from './headers/ungrouped_header';
import { DEFAULT_ALERT_TAB, useFilterQuery, useSelectedTab } from './hooks';

export interface AlertGroup {
  id: string;
  updated?: string;
  updatedBy?: string | undefined;
  name: string;
  statusMessage: string;
  alertKeys: string[];
  bugs: string[];
}

export const Alerts = () => {
  const tree = useTree();
  const [filter] = useFilterQuery('');
  let {
    builderAlerts,
    stepAlerts,
    testAlerts,
    alertsLoading,
    alertsLoadingStatus,
  } = useAlerts();
  const [organizeBy, setOrganizeBy] = useState<AlertKind>('builder');
  const [showResolved, setShowResolved] = useState(false);
  const [selectedTab, setSelectedTab] = useSelectedTab(DEFAULT_ALERT_TAB);
  const [alertGroups, setAlertGroups] = useState<AlertGroup[]>([]);

  if (!tree) {
    return <></>;
  }

  // Filter out all alerts based on flaky/resolved status.
  if (!showResolved) {
    builderAlerts = builderAlerts.filter((a) => a.consecutiveFailures > 1);
    stepAlerts = stepAlerts.filter((a) => a.consecutiveFailures > 1);
    testAlerts = testAlerts.filter((a) => a.consecutiveFailures > 1);
  }

  let topLevelAlerts: GenericAlert[] = builderAlerts;
  if (organizeBy === 'step') {
    topLevelAlerts = stepAlerts;
  } else if (organizeBy === 'test') {
    topLevelAlerts = testAlerts;
  }
  const ungroupedTopLevelAlerts = topLevelAlerts.filter(
    (a) => !alertGroups.some((g) => g.alertKeys.includes(a.key)),
  );

  let selectedGroup = undefined;
  let header = null;
  let selectedTopLevelAlerts = topLevelAlerts;
  if (selectedTab === 'ungrouped') {
    selectedTopLevelAlerts = ungroupedTopLevelAlerts;
    header = <UngroupedHeader />;
  } else if (selectedTab === 'all') {
    selectedTopLevelAlerts = topLevelAlerts;
    header = <AllHeader />;
  } else if (selectedTab.startsWith('group:')) {
    const groupIndex = alertGroups.findIndex(
      (g) => g.id === selectedTab.slice(6),
    );
    const group = alertGroups[groupIndex];
    const setGroup = (group: AlertGroup) => {
      alertGroups[groupIndex] = group;
      setAlertGroups([...alertGroups]);
    };
    const archiveGroup = () => {
      alertGroups[groupIndex] = group;
      setAlertGroups([
        ...alertGroups.slice(0, groupIndex),
        ...alertGroups.slice(groupIndex + 1),
      ]);
    };
    if (!group) {
      selectedTopLevelAlerts = [];
    } else {
      selectedGroup = group;
      header = (
        <GroupHeader
          group={group}
          setGroup={setGroup}
          archiveGroup={archiveGroup}
        />
      );
      selectedTopLevelAlerts = group.alertKeys
        .map(
          (key) =>
            builderAlerts.find((a) => a.key === key) ||
            stepAlerts.find((a) => a.key === key) ||
            testAlerts.find((a) => a.key === key),
        )
        .filter((a) => !!a);
    }
  }

  const structuredAlerts = buildStructuredAlertsPreSorted(
    selectedTopLevelAlerts,
    builderAlerts,
    stepAlerts,
    testAlerts,
  );
  const selectedAlerts = filterAlerts(structuredAlerts, filter);

  return (
    <Box sx={{ display: 'flex' }}>
      <Box sx={{ minWidth: '256px', flexGrow: 1, flexShrink: 0 }}>
        <AlertsSideNav
          tree={tree}
          selectedTab={selectedTab}
          setSelectedTab={setSelectedTab}
          topLevelAlerts={topLevelAlerts}
          ungroupedTopLevelAlerts={ungroupedTopLevelAlerts}
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
            <Tooltip
              title={`${showResolved ? 'Hide' : 'Show'} flaky and resolved alerts`}
            >
              <Button
                variant="outlined"
                color={showResolved ? 'primary' : 'inherit'}
                onClick={() => setShowResolved(!showResolved)}
              >
                <VisibilityIcon />
              </Button>
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
          alerts={selectedAlerts}
          group={selectedGroup}
          groups={alertGroups}
          setGroups={setAlertGroups}
        />
      </Box>
    </Box>
  );
};

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
