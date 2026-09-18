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

import { generateChromeOsDeviceDetailsURL } from '@/fleet/constants/paths';
import { FLEET_BUILDS_SWARMING_HOST } from '@/fleet/utils/builds';
import { md, rawMd, toFullUrl } from '@/fleet/utils/markdown_utils';
import { ScheduleAutorepairResult } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

/**
 * Generates a formatted Markdown summary of autorepair results suitable for
 * pasting into Buganizer issues and comments. Includes full clickable URLs
 * for each device, its Milo task (or error message), and the Swarming session.
 */
export function generateAutorepairBugMarkdown(
  results: readonly ScheduleAutorepairResult[] = [],
  sessionId?: string,
): string {
  if (results.length === 0 && !sessionId) {
    return '';
  }

  const lines: string[] = ['**Autorepair results:**'];

  for (const result of results) {
    const unitName = result.unitName ?? '';
    const safeUnitName = rawMd(unitName.replace(/([\[\]\\])/g, '\\$1'));
    const deviceUrl = rawMd(
      toFullUrl(generateChromeOsDeviceDetailsURL(unitName)),
    );
    if (result.taskUrl) {
      const miloUrl = rawMd(toFullUrl(result.taskUrl));
      lines.push(
        md`* [${safeUnitName}](${deviceUrl}): [View in Milo](${miloUrl})`,
      );
    } else {
      const errorMsg = result.errorMessage || 'Unknown error';
      lines.push(
        md`* [${safeUnitName}](${deviceUrl}): Failed to schedule autorepair: ${errorMsg}`,
      );
    }
  }

  if (sessionId) {
    const swarmingUrl = rawMd(
      `https://${FLEET_BUILDS_SWARMING_HOST}/tasklist?f=admin-session:${encodeURIComponent(sessionId)}`,
    );
    lines.push('', md`[View tasks in Swarming](${swarmingUrl})`);
  }

  return lines.join('\n');
}
