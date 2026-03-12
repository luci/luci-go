// Copyright 2026 The LUCI Authors.
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

export const generateIssueDescription = (dutInfo: string) => {
  const description = [
    'Fleet Operations will repair DUTS that are in "needs_manual_repair" status. ' +
      'repair_failed & needs_repair are being recovered by auto repair tasks. ' +
      'You can submit a bug for DUTs in a different state if you suspect that the state is incorrect, ' +
      'or something is wrong with devices peripherals. ' +
      'Please do not explicitly assign bugs to individuals without prior ' +
      'discussion with said individuals.',
    '------------------------------------------------------------------------------------',
    '',
    '**DUT Link(s) / Locations:**:',
    '',
    `${dutInfo}`,
    '',
    '**Issue:**',
    '',
    '<Please provide a summary of the issue.>',
    '',
    '**Action Item:**',
    '',
    '**Logs (if applicable):**',
  ].join('\n');
  return encodeURIComponent(description);
};
