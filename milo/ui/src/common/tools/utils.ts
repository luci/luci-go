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

/**
 * Generates URL for collecting feedback.
 */
export function genFeedbackUrl(errMsg?: string, stacktrace?: string) {
  const feedbackComment =
    `Version: ${UI_VERSION}\n` +
    `From Link: ${self.location.href}\n` +
    (errMsg ? `Error Message:\n${errMsg}\n` : '') +
    (stacktrace ? `Stacktrace:\n${stacktrace}\n` : '') +
    'Please enter a description of the problem, with steps to reproduce if applicable.';

  const searchParams = new URLSearchParams({
    // Public Trackers > Chromium Public Trackers > Chromium > Infra > LUCI > UserInterface
    component: '1456503',
    type: 'BUG',
    priority: 'P2',
    severity: 'S2',
    inProd: 'true',
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
