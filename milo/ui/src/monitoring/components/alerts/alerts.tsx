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

import SearchIcon from '@mui/icons-material/Search';
import { InputAdornment, TextField, Typography } from '@mui/material';
import { useState } from 'react';

import {
  AlertJson,
  AnnotationJson,
  TreeJson,
  BugId,
  Bug,
  bugFromId,
} from '@/monitoring/util/server_json';

import { AlertGroup } from './alert_group';
import { BugGroup } from './bug_group';

interface AlertsProps {
  tree: TreeJson;
  alerts: AlertJson[] | undefined | null;
  bugs: Bug[];
  annotations: { [key: string]: AnnotationJson };
}
export const Alerts = ({ tree, alerts, bugs, annotations }: AlertsProps) => {
  const [filter, setFilter] = useState('');
  if (!tree || !alerts) {
    return <></>;
  }
  const filtered = filterAlerts(alerts, filter);
  const categories = categorizeAlerts(filtered, annotations);

  // Add any bugs associated with issues that are not in the hotlist.
  // TODO: This probably needs to be pushed higher up so the details can be queried from Buganizer.
  for (const bug of Object.keys(categories.bugAlerts)) {
    if (bugs.filter((b) => b.number == bug).length == 0) {
      bugs.push(bugFromId(bug));
    }
  }

  return (
    <>
      <TextField
        id="input-with-icon-textfield"
        placeholder="Filter Alerts and Bugs"
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <SearchIcon />
            </InputAdornment>
          ),
        }}
        variant="outlined"
        fullWidth
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
      />
      <AlertGroup
        groupName={'Consistent Failures'}
        alertBugs={categories.alertBugs}
        alerts={categories.consistentFailures}
        tree={tree}
        groupDescription="Failures that have occurred at least 2 times in a row and are not linked with a bug"
        defaultExpanded={true}
        bugs={bugs}
      />
      <AlertGroup
        groupName={'New Failures'}
        alertBugs={categories.alertBugs}
        alerts={categories.newFailures}
        tree={tree}
        groupDescription="Failures that have only been seen once and are not linked with a bug"
        defaultExpanded={false}
        bugs={bugs}
      />

      <Typography variant="h6" sx={{ margin: '24px 0 8px' }}>
        Bugs
      </Typography>
      {/* TODO: Get hotlist name */}
      {bugs.length == 0 ? (
        <Typography>There are currently no bugs in the hotlist.</Typography>
      ) : null}
      {bugs.map((bug) => {
        const numAlerts = categories.bugAlerts[bug.number]?.length || 0;
        if (filter != '' && numAlerts == 0) {
          return null;
        }
        return (
          <BugGroup
            key={bug.link}
            bug={bug}
            alertBugs={categories.alertBugs}
            alerts={categories.bugAlerts[bug.number]}
            tree={tree}
            bugs={bugs}
          />
        );
      })}
      {filter != '' && Object.keys(categories.bugAlerts).length == 0 && (
        <Typography>
          No alerts associated with bugs match your search filter, try changing
          or removing the search filter.
        </Typography>
      )}
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
  // All the bugs associated with each alert.
  alertBugs: { [alertKey: string]: BugId[] };
}

// Sort alerts into categories - one for each bug, and the leftovers into
// either consistent (multiple failures) or new (a single failure).
const categorizeAlerts = (
  alerts: AlertJson[],
  annotations: { [key: string]: AnnotationJson },
): CategorizedAlerts => {
  const categories: CategorizedAlerts = {
    consistentFailures: [],
    newFailures: [],
    bugAlerts: {},
    alertBugs: {},
  };
  if (annotations) {
    for (const alert of alerts) {
      const annotation = annotations[alert.key];
      for (const b of annotation?.bugs || []) {
        categories.bugAlerts[b.id] = categories.bugAlerts[b.id] || [];
        categories.bugAlerts[b.id].push(alert);
        categories.alertBugs[alert.key] = categories.alertBugs[alert.key] || [];
        categories.alertBugs[alert.key].push(b);
      }
    }
  }
  for (const alert of alerts) {
    if (categories.alertBugs[alert.key]) {
      continue;
    }
    const builder = alert.extension?.builders?.[0];
    const failureCount =
      builder && builder.first_failure_build_number == 0
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
  const re = new RegExp(filter);
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
