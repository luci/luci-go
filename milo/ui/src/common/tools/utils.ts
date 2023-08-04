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
    `Version: ${CONFIGS.VERSION}\n` +
    `From Link: ${self.location.href}\n` +
    (errMsg ? `Error Message:\n${errMsg}\n` : '') +
    (stacktrace ? `Stacktrace:\n${stacktrace}\n` : '') +
    'Please enter a description of the problem, with repro steps if applicable.';

  const searchParams = new URLSearchParams({
    template: 'Build Infrastructure',
    components: 'Infra>LUCI>UserInterface',
    labels: 'Pri-2,Type-Bug',
    comment: feedbackComment,
  });
  return `https://bugs.chromium.org/p/chromium/issues/entry?${searchParams}`;
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
