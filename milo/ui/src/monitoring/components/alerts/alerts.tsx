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

import {
  Alert,
  Box,
  CircularProgress,
  LinearProgress,
  Typography,
} from '@mui/material';

import { SearchInput } from '@/common/components/search_input';
import {
  useAlerts,
  useTree,
  useBugs,
} from '@/monitoring/pages/monitoring_page/context';
import { AlertJson } from '@/monitoring/util/server_json';

import { AlertGroup } from './alert_group';
import { BugGroup } from './bug_group';
import { useFilterQuery } from './hooks';
import { TreeStatusCard } from './tree_status_card';

function SearchAlertInput() {
  const [filter, updateFilter] = useFilterQuery('');
  return (
    <SearchInput
      initDelayMs={200}
      onValueChange={(value) => updateFilter(value)}
      size="small"
      value={filter}
      placeholder="Filter Alerts - hint: use this to perform operations like linking all matching alerts to a bug"
    />
  );
}

export const Alerts = () => {
  const [filter] = useFilterQuery('');
  const { alerts, alertsLoading } = useAlerts();
  const tree = useTree();
  const { bugs, bugsLoading, bugsError, isBugsError } = useBugs();

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

  if (alertsLoading) {
    return <CircularProgress />;
  }

  const bugsErrorMessage =
    bugsError && bugsError instanceof Error ? bugsError.message : bugsError;
  return (
    <>
      <Box
        sx={{
          position: 'sticky',
          top: 48,
          zIndex: 30,
          backgroundColor: '#fff',
          padding: '8px 16px',
          boxShadow: 3,
        }}
      >
        <SearchAlertInput />
      </Box>
      <div style={{ margin: '16px 0', padding: '0 16px' }}>
        <TreeStatusCard tree={tree} />
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

        <Typography variant="h6" sx={{ margin: '24px 0 8px' }}>
          Bugs
        </Typography>
        {bugsLoading ? <LinearProgress /> : null}
        {isBugsError ? (
          <Alert severity="error">
            Failed to fetch bugs: {`${bugsErrorMessage}`}
          </Alert>
        ) : null}
        {/* TODO: Get hotlist name */}
        {bugs?.length === 0 ? (
          <Typography>There are currently no bugs in the hotlist.</Typography>
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
      </div>
    </>
  );
};

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
