// Copyright 2025 The LUCI Authors.
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

import { Link, Typography } from '@mui/material';

import { InfoTooltip } from '../info_tooltip/info_tooltip';

/**
 * Renders an interactive hovercard.
 * Uses a timer-based delay to allow the user's mouse to travel from the
 * trigger icon to the popover content without it closing prematurely.
 */
export function LeaseStateInfo() {
  return (
    <InfoTooltip>
      <>
        <Typography variant="body2">
          A device is considered &quot;<b>Leased</b>&quot; if it is currently in
          use by a leasing client such as Swarming. Devices may be leased for
          usage in both automated and manual testing.
        </Typography>
        <Typography variant="caption" component="div" sx={{ mt: 2 }}>
          [1] Traditionally humans would lease ChromeOS devices through the{' '}
          <Link
            href="http://go/crosfleet-cli#dut-lease"
            target="_blank"
            rel="noreferrer"
          >
            <b>crosfleet</b>
          </Link>
          . The term &quot;lease&quot; as applied by <b>crosfleet</b> refers to
          the process of having a human lease a device while the Fleet
          Console&apos;s leasing concept is more generic and tracks whether a
          device has been leased by either a human or an automated client.
        </Typography>
      </>
    </InfoTooltip>
  );
}
