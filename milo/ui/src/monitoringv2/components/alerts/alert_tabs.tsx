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

import { TabContext, TabPanel } from '@mui/lab';
import {
  Alert,
  Box,
  CircularProgress,
  Link,
  Tab,
  Tabs,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import { useEffect, useState } from 'react';

import {
  useAlerts,
  useBugs,
  useTree,
} from '@/monitoringv2/pages/monitoring_page/context';
import {
  GenericAlert,
  BuilderAlert,
  StepAlert,
  TestAlert,
  AlertKind,
} from '@/monitoringv2/pages/monitoring_page/context/context';

import { AlertGroup } from './alert_group';
import { BugGroup } from './bug_group';
import { DEFAULT_ALERT_TAB, useFilterQuery, useSelectedTab } from './hooks';

export function AlertTabs() {
  const [filter] = useFilterQuery('');
  const {
    alerts,
    builderAlerts,
    stepAlerts,
    testAlerts,
    alertsLoading,
    alertsLoadingStatus,
  } = useAlerts();
  const { bugs, bugsLoading, bugsError, isBugsError } = useBugs();
  const tree = useTree();
  const [selectedTab, setSelectedTab] = useSelectedTab();
  const [organizeBy, setOrganizeBy] = useState('builder' as AlertKind);
  const [showResolved, setShowResolved] = useState(false);

  // This should be only called once to select the default tab.
  useEffect(() => {
    if (selectedTab === null) {
      setSelectedTab(DEFAULT_ALERT_TAB);
    }
  }, [selectedTab, setSelectedTab]);

  if (!tree) {
    return <></>;
  }

  const organizedAlerts = organizeAlerts(
    organizeBy,
    showResolved,
    builderAlerts,
    stepAlerts,
    testAlerts,
  );
  const filtered = filterAlerts(organizedAlerts, filter);
  const unfilteredCategories = categorizeAlerts(organizedAlerts || []);
  const categories = categorizeAlerts(filtered);

  const bugsWithAlerts =
    (alerts && bugs?.filter((b) => alerts.some((a) => a.bug === b.number))) ||
    [];
  const bugsWithoutAlerts =
    (alerts && bugs?.filter((b) => !alerts.some((a) => a.bug === b.number))) ||
    [];

  const bugsErrorMessage =
    bugsError && bugsError instanceof Error ? bugsError.message : bugsError;

  function handleTabChange(_: React.SyntheticEvent, newValue: string) {
    setSelectedTab(newValue);
  }

  if (alertsLoading) {
    return (
      <>
        <CircularProgress />
        <Typography>{alertsLoadingStatus}</Typography>
      </>
    );
  }

  return (
    <TabContext value={selectedTab || DEFAULT_ALERT_TAB}>
      <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
        <Tabs
          value={selectedTab || DEFAULT_ALERT_TAB}
          onChange={handleTabChange}
          aria-label="Alerts tabs"
        >
          <Tab value="untriaged" label="Untriaged" />
          <Tab value="bugs" label="Bugs" />
        </Tabs>
      </Box>
      <TabPanel value="untriaged">
        <Box sx={{ display: 'flex', gap: '10px', alignItems: 'center' }}>
          <label htmlFor="organizeByGroup">Organize By: </label>
          <ToggleButtonGroup
            id="organizeByGroup"
            exclusive
            value={organizeBy}
            onChange={(_, v) => setOrganizeBy(v)}
          >
            <ToggleButton value="builder">Builder</ToggleButton>
            <ToggleButton value="step">Step</ToggleButton>
            <ToggleButton value="test">Test</ToggleButton>
          </ToggleButtonGroup>
          <FormControlLabel
            control={
              <Switch
                checked={showResolved}
                onChange={(event: React.ChangeEvent<HTMLInputElement>) =>
                  setShowResolved(event.target.checked)
                }
              />
            }
            label="Show Resolved"
          />
        </Box>
        <AlertGroup
          groupName={'Consistent Failures'}
          alerts={categories.consistentFailures}
          hiddenAlertsCount={
            unfilteredCategories.consistentFailures.length -
            categories.consistentFailures.length
          }
          tree={tree}
          groupDescription="Failures that have occurred at least 2 times in a row"
          defaultExpanded={true}
          bugs={bugs || []}
        />
        <AlertGroup
          groupName={'New Failures'}
          alerts={categories.newFailures}
          hiddenAlertsCount={
            unfilteredCategories.consistentFailures.length -
            categories.newFailures.length
          }
          tree={tree}
          groupDescription="Failures that have only been seen once"
          defaultExpanded={false}
          bugs={bugs || []}
        />
      </TabPanel>
      <TabPanel value="bugs">
        {bugsLoading ? <CircularProgress sx={{ margin: '10px 0' }} /> : null}
        {isBugsError ? (
          <Alert severity="error">
            Failed to fetch bugs: {`${bugsErrorMessage}`}
          </Alert>
        ) : null}
        {/* TODO: Get hotlist name */}
        {bugs?.length === 0 ? (
          <Typography>
            {tree.hotlistId ? (
              <>
                There are currently no alerts associated with bugs and the{' '}
                <Link
                  href={`https://issuetracker.google.com/hotlists/${tree.hotlistId}`}
                  target="_blank"
                  rel="noreferrer"
                >
                  hotlist
                </Link>
                associated with this tree is empty.
              </>
            ) : (
              'There are no alerts associated with bugs and there is no hotlist associated with this tree.'
            )}
          </Typography>
        ) : null}
        {bugsWithAlerts.map((bug) => {
          const numAlerts = categories.bugAlerts[bug.number]?.length || 0;
          if (filter !== '' && numAlerts === 0) {
            return null;
          }
          return (
            <BugGroup
              key={bug.link}
              bug={bug}
              alerts={categories.bugAlerts[bug.number]}
              tree={tree}
              bugs={bugs || []}
            />
          );
        })}
        {bugsWithoutAlerts.map((bug) => {
          const numAlerts = categories.bugAlerts[bug.number]?.length || 0;
          if (filter !== '' && numAlerts === 0) {
            return null;
          }
          return (
            <BugGroup
              key={bug.link}
              bug={bug}
              alerts={categories.bugAlerts[bug.number]}
              tree={tree}
              bugs={bugs || []}
            />
          );
        })}
        {filter !== '' && Object.keys(categories.bugAlerts).length === 0 && (
          <Typography>
            No alerts associated with bugs match your search filter, try
            changing or removing the search filter.
          </Typography>
        )}
        <Typography sx={{ opacity: '70%', marginTop: '40px' }}>
          {tree.hotlistId ? (
            <>
              This view displays both bugs associated with current alerts and
              the bugs in the{' '}
              <Link
                href={`https://issuetracker.google.com/hotlists/${tree.hotlistId}`}
                target="_blank"
                rel="noreferrer"
              >
                hotlist
              </Link>{' '}
              associated with this tree.
            </>
          ) : (
            'This view displays bugs associated with current alerts. There is no hotlist associated with this tree.'
          )}
        </Typography>
      </TabPanel>
    </TabContext>
  );
}

export interface StructuredAlert {
  alert: GenericAlert;
  children: StructuredAlert[];
  consecutiveFailures: number;
  consecutivePasses: number;
}

// exported for testing only.
export const organizeAlerts = (
  kind: AlertKind,
  showResolved: boolean,
  builderAlerts: BuilderAlert[],
  stepAlerts: StepAlert[],
  testAlerts: TestAlert[],
): StructuredAlert[] => {
  if (!showResolved) {
    builderAlerts = builderAlerts.filter((a) => a.consecutiveFailures > 0);
    stepAlerts = stepAlerts.filter((a) => a.consecutiveFailures > 0);
    testAlerts = testAlerts.filter((a) => a.consecutiveFailures > 0);
  }
  if (kind === 'builder') {
    return organizeRelatedAlerts([builderAlerts, stepAlerts, testAlerts]);
  } else if (kind === 'step') {
    return organizeRelatedAlerts([stepAlerts, builderAlerts, testAlerts]);
  } else if (kind === 'test') {
    return organizeRelatedAlerts([testAlerts, stepAlerts, builderAlerts]);
  }
  return [];
};

const organizeRelatedAlerts = (
  alertGroups: GenericAlert[][],
): StructuredAlert[] => {
  if (alertGroups.length === 0) {
    // Should never happen.
    return [];
  }
  const alerts = alertGroups[0].map(makeStructuredAlert);
  if (alertGroups.length > 1) {
    alerts.forEach((alert) => {
      alert.children = sortAlertsByFailurePattern(
        organizeRelatedAlerts([
          alertGroups[1].filter((a) => isAlertRelated(a, alert.alert)),
          ...alertGroups.slice(2),
        ]),
        alert.consecutiveFailures,
      );
    });
  }
  return alerts;
};

const isAlertRelated = (a: GenericAlert, b: GenericAlert): boolean => {
  return a.key.startsWith(b.key) || b.key.startsWith(a.key);
};

const makeStructuredAlert = (alert: GenericAlert): StructuredAlert => {
  return {
    alert,
    children: [],
    consecutiveFailures: alert.consecutiveFailures,
    consecutivePasses: alert.consecutivePasses,
  };
};

/**
 * sortAlertsByFailurePattern attempts to sort child alerts in the order most interesting to
 * a user looking at them.
 *
 * First present any alerts where the number of consecutive failures matches the parent alert
 * exactly.  These are most likely to be the cause of the parent failure.
 *
 * Next present any alerts where the number of consecustive failures is less than the parent,
 * in descending order of the consecutive failures.  If there are no exact matches these are
 * most likely to be the cause (with earlier failures having a different cause).
 *
 * Next present any alerts with a larger number of consecutive failures than the parent.  These
 * are unlikely to be the cause because the parent passed when these were failing.
 *
 * Last present any alerts that have zero consecutive failures, these are already resolved and
 * Are likely only of interest for verifying a fix.
 *
 * For any alerts that are equal under the above rules, break ties with a localeCompare of the alert keys.
 *
 * @param alerts The child alerts to sort
 * @param parentConsecutiveFailures The number of consecutive failures observed in the parent of these alerts.
 * @returns The child alerts sorted as described above.
 */
const sortAlertsByFailurePattern = (
  alerts: StructuredAlert[],
  parentConsecutiveFailures: number,
): StructuredAlert[] => {
  return alerts.sort((a, b) => {
    if (a.consecutiveFailures === b.consecutiveFailures) {
      if (a.consecutiveFailures === 0) {
        return a.consecutivePasses === b.consecutivePasses
          ? a.alert.key.localeCompare(b.alert.key)
          : a.consecutivePasses - b.consecutivePasses;
      }
      return a.alert.key.localeCompare(b.alert.key);
    }
    if (a.consecutiveFailures === parentConsecutiveFailures) {
      return b.consecutiveFailures === parentConsecutiveFailures
        ? a.alert.key.localeCompare(b.alert.key)
        : -1;
    } else if (b.consecutiveFailures === parentConsecutiveFailures) {
      return 1;
    }

    if (a.consecutiveFailures < parentConsecutiveFailures) {
      return b.consecutiveFailures < parentConsecutiveFailures
        ? b.consecutiveFailures - a.consecutiveFailures
        : -1;
    } else if (b.consecutiveFailures < parentConsecutiveFailures) {
      return 1;
    }

    return a.consecutiveFailures - b.consecutiveFailures;
  });
};

interface CategorizedAlerts {
  // Alerts not associated with a bug that have occurred in more than one consecutive build.
  consistentFailures: StructuredAlert[];
  // Alerts not associated with bugs that have only happened once.
  newFailures: StructuredAlert[];
  bugAlerts: { [bug: string]: StructuredAlert[] };
}

// Sort alerts into categories - one for each bug, and the leftovers into
// either consistent (multiple failures) or new (a single failure).
const categorizeAlerts = (alerts: StructuredAlert[]): CategorizedAlerts => {
  const categories: CategorizedAlerts = {
    consistentFailures: [],
    newFailures: [],
    bugAlerts: {},
  };
  for (const alert of alerts) {
    if (alert.consecutiveFailures <= 1) {
      categories.newFailures.push(alert);
    } else {
      categories.consistentFailures.push(alert);
    }
  }
  return categories;
};

// filterAlerts returns the alerts that match the given filter string typed by the user.
// alerts can match in step name, builder name, test id, or whatever else is useful to users.
const filterAlerts = (
  alerts: StructuredAlert[],
  filter: string,
): StructuredAlert[] => {
  if (filter === '') {
    return alerts;
  }
  const re = new RegExp(filter, 'i');
  return alerts.filter((alert) => {
    const b = alert.alert.builderID;
    if (re.test(b.bucket) || re.test(b.builder)) {
      return true;
    }
    return filterAlerts(alert.children, filter).length > 0;
  });
};
