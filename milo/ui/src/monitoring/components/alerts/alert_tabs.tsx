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
  Typography,
} from '@mui/material';
import { useEffect } from 'react';

import {
  useAlerts,
  useBugs,
  useTree,
} from '@/monitoring/pages/monitoring_page/context';
import { AlertJson } from '@/monitoring/util/server_json';

import { AlertGroup } from './alert_group';
import { BugGroup } from './bug_group';
import { DEFAULT_ALERT_TAB, useFilterQuery, useSelectedTab } from './hooks';

export function AlertTabs() {
  const [filter] = useFilterQuery('');
  const { alerts, alertsLoading } = useAlerts();
  const { bugs, bugsLoading, bugsError, isBugsError } = useBugs();
  const tree = useTree();
  const [selectedTab, setSelectedTab] = useSelectedTab();

  // This should be only called once to select the default tab.
  useEffect(() => {
    if (selectedTab === null) {
      setSelectedTab(DEFAULT_ALERT_TAB);
    }
  }, [selectedTab, setSelectedTab]);

  if (!tree) {
    return <></>;
  }

  const filtered = filterAlerts(alerts || [], filter);
  const unfilteredCategories = categorizeAlerts(alerts || []);
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
    return <CircularProgress />;
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
        <AlertGroup
          groupName={'Untriaged Consistent Failures'}
          alerts={categories.consistentFailures}
          hiddenAlertsCount={
            unfilteredCategories.consistentFailures.length -
            categories.consistentFailures.length
          }
          tree={tree}
          groupDescription="Failures that have occurred at least 2 times in a row and are not linked with a bug"
          defaultExpanded={true}
          bugs={bugs || []}
        />
        <AlertGroup
          groupName={'Untriaged New Failures'}
          alerts={categories.newFailures}
          hiddenAlertsCount={
            unfilteredCategories.consistentFailures.length -
            categories.newFailures.length
          }
          tree={tree}
          groupDescription="Failures that have only been seen once and are not linked with a bug"
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

interface CategorizedAlerts {
  // Alerts not associated with a bug that have occurred in more than one consecutive build.
  consistentFailures: AlertJson[];
  // Alerts not associated with bugs that have only happened once.
  newFailures: AlertJson[];
  // All the alerts assoctiated with each bug.
  bugAlerts: { [bug: string]: AlertJson[] };
}

// Sort alerts into categories - one for each bug, and the leftovers into
// either consistent (multiple failures) or new (a single failure).
const categorizeAlerts = (alerts: AlertJson[]): CategorizedAlerts => {
  const categories: CategorizedAlerts = {
    consistentFailures: [],
    newFailures: [],
    bugAlerts: {},
  };
  for (const alert of alerts) {
    if (alert.bug) {
      categories.bugAlerts[alert.bug] = categories.bugAlerts[alert.bug] || [];
      categories.bugAlerts[alert.bug].push(alert);
      continue;
    }
    const builder = alert.extension?.builders?.[0];
    const failureCount =
      builder && builder.first_failure_build_number === 0
        ? undefined
        : builder.latest_failure_build_number -
          builder.first_failure_build_number +
          1;
    const isNewFailure = failureCount === 1;
    if (isNewFailure) {
      categories.newFailures.push(alert);
    } else {
      categories.consistentFailures.push(alert);
    }
  }
  return categories;
};

// filterAlerts returns the alerts that match the given filter string typed by the user.
// alerts can match in step name, builder name, test id, or whatever else is useful to users.
const filterAlerts = (alerts: AlertJson[], filter: string): AlertJson[] => {
  if (filter === '') {
    return alerts;
  }
  const re = new RegExp(filter, 'i');
  return alerts.filter((alert) => {
    if (
      alert.extension.builders.filter(
        (b) => re.test(b.bucket) || re.test(b.builder_group) || re.test(b.name),
      ).length > 0
    ) {
      return true;
    }
    if (
      alert.extension.reason?.tests?.filter(
        (t) => re.test(t.test_id) || re.test(t.test_id),
      ).length > 0
    ) {
      return true;
    }
    return re.test(alert.title) || re.test(alert.extension.reason?.step);
  });
};
