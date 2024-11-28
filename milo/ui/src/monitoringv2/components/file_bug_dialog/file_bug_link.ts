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

import { TreeJson } from '@/monitoringv2/util/server_json';

export const fileBugLink = (tree: TreeJson, alerts: string[]): string => {
  const projectId = monorailProjectId(tree);
  // FIXME!
  const summary = `${alerts[0]} failing`; // bugSummary(tree, alerts);
  const description = `The following builders/steps/tests are failing:\n- ${alerts.map((a) => a).join('\n- ')}\n`;
  // bugComment(tree, alerts);
  const labels = bugLabels(tree);
  return `https://bugs.chromium.org/p/${encodeURIComponent(
    projectId,
  )}/issues/entry?summary=${encodeURIComponent(
    summary,
  )}&description=${encodeURIComponent(description)}&labels=${encodeURIComponent(
    labels.join(','),
  )}`;
};

// const bugSummary = (tree: TreeJson, alerts: AlertJson[]): string => {
//   let bugSummary = 'Bug filed from LUCI Monitoring';
//   if (alerts && alerts.length) {
//     if (
//       tree.name === 'android' &&
//       alerts[0].extension?.builders?.length === 1 &&
//       alertIsTestFailure(alerts[0])
//     ) {
//       bugSummary = `<insert test name/suite> is failing on builder "${alerts[0].extension.builders[0].name}"`;
//     } else {
//       bugSummary = alerts[0].title;
//     }
//     if (alerts.length > 1) {
//       bugSummary += ` and ${alerts.length - 1} other alerts`;
//     }
//   }
//   return bugSummary;
// };

const monorailProjectId = (tree: TreeJson): string => {
  return tree.default_monorail_project_name || 'chromium';
};

// const alertIsTestFailure = (alert: AlertJson): boolean => {
//   return (
//     alert.type === 'test-failure' ||
//     alert.extension?.reason?.step?.includes('test')
//   );
// };

// const bugComment = (tree: TreeJson, alerts: AlertJson[]): string => {
//   return alerts.reduce((comment, alert) => {
//     let result = '';
//     if (alert.extension?.builders?.length > 0) {
//       const isTestFailure = alertIsTestFailure(alert);
//       if (
//         alert.extension.builders.length === 1 &&
//         isTestFailure &&
//         tree.name === 'android'
//       ) {
//         const step = alert.extension.reason.step;
//         const builder = alert.extension.builders[0];
//         result += `<insert test name/suite> is failing in step "${step}" on builder "${builder.name}"\n\n`;
//       } else {
//         result += alert.title + '\n\n';
//       }
//       const failuresInfo: string[] = [];
//       for (const builder of alert.extension.builders) {
//         failuresInfo.push(builderFailureInfo(builder));
//       }
//       result +=
//         'List of failed builders:\n\n' +
//         failuresInfo.join('\n--------------------\n') +
//         '\n';

//       if (tree.name === 'android') {
//         result += `
// ------- Note to sheriffs -------

// For failing tests:
// Please file a separate bug for each failing test suite, filling in the name of the test or suite (<in angle brackets>).

// Add a component so that bugs end up in the appropriate triage queue, and assign an owner if possible.

// If applicable, also include a sample stack trace, link to the flakiness dashboard, and/or post-test screenshot to help with
// future debugging.

// If a culprit CL can be identified, revert the CL. Otherwise, disable the test.
// When either action is complete and the issue no longer requires sheriff attention, remove the ${tree.bug_queue_label} label.

// For infra failures:
// See go/bugatrooper for instructions and bug templates

// ------------------------------
// `;
//       }
//     }
//     return comment + result;
//   }, '');
// };

// const builderFailureInfo = (builder: AlertBuilderJson): string => {
//   let s = 'Builder: ' + builder.name;
//   s += '\n' + builder.url;
//   if (builder.first_failure_url) {
//     s += '\n' + 'First failing build:';
//     s += '\n' + builder.first_failure_url;
//   } else if (builder.latest_failure_url) {
//     s += '\n' + 'Latest failing build:';
//     s += '\n' + builder.latest_failure_url;
//   }
//   return s;
// };

const bugLabels = (tree: TreeJson): string[] => {
  const labels = ['Filed-Via-LUCI-Monitoring'];
  if (!tree) {
    return labels;
  }

  if (tree.name === 'android') {
    labels.push('Restrict-View-Google');
  }
  if (tree.bug_queue_label) {
    labels.push(tree.bug_queue_label);
  }
  return labels;
};
