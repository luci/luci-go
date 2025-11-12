// Copyright 2023 The LUCI Authors.
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

import { getEnabledFeatureFlags } from '@/fleet/config/features';

export type GenFeedbackUrlArgs =
  | {
      errMsg?: string;
      stacktrace?: string;
      bugComponent?: string;
      customComment?: string;
    }
  | undefined;
/**
 * Generates URL for collecting feedback.
 */
export function genFeedbackUrl({
  errMsg,
  stacktrace,
  bugComponent,
  customComment,
}: GenFeedbackUrlArgs = {}) {
  const feedbackComment =
    customComment ||
    `# Problem Description\n\n` +
      'Please enter a description of the problem.\n\n' +
      '# Steps to Reproduce\n\n' +
      'Please provide steps to reproduce the problem.\n\n' +
      '1. \n\n' +
      '2. \n\n' +
      '3. \n\n' +
      '# Network Calls\n\n' +
      `It would be very helpful if you can take a screenshot of your ` +
      `[network tab](https://developer.chrome.com/docs/devtools/network).\n` +
      `**Note that you should open your network tab before you load the page.**\n` +
      `Only network calls occurred after you open the network tab are recorded.\n\n` +
      '# Autopopulated Info\n\n' +
      `- **Version**: ${UI_VERSION}\n` +
      `- **From Link**: ${self.location.href}\n` +
      `- **User Agent**: ${navigator.userAgent}\n` +
      `- **Enabled Feature Flags**: ${getEnabledFeatureFlags().join(', ') || 'None'}\n\n` +
      (errMsg ? `- **Error Message**:\n  ${errMsg}\n\n` : '') +
      (stacktrace ? `- **Stacktrace**:\n  ${stacktrace}\n\n` : '');

  const searchParams = new URLSearchParams({
    // Public Trackers > Chromium Public Trackers > Chromium > Infra > LUCI > UserInterface
    component: bugComponent ?? '1456503',
    type: 'BUG',
    priority: 'P2',
    severity: 'S2',
    inProd: 'true',
    format: 'MARKDOWN',
    description: feedbackComment,
  });
  return `https://issuetracker.google.com/issues/new?${searchParams}`;
}

/**
 * Extract project from a string which can be a realm or a project.
 *
 * @param projectOrRealm a LUCI project name or a realm
 * @returns a LUCI project name
 */
export function extractProject(projectOrRealm: string): string {
  return projectOrRealm.split(':', 2)[0];
}
