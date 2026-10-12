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

import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { Tooltip } from '@mui/material';
import { Link } from 'react-router';

import { BuganizerLink } from '@/fleet/components/table/buganizer_link';
import { generateDeviceDetailsURL } from '@/fleet/constants/paths';
import { getMobileHarnessDeviceListLink } from '@/fleet/utils/mobile_harness';
import { AndroidPageWorkspace, workspaces } from '@/fleet/workspaces';

export interface AndroidDeviceIdCellProps {
  readonly deviceId: string;
  readonly workspace: AndroidPageWorkspace;
  /**
   * Mobile Harness page to open. Defaults to the Mobile Harness device list
   * filtered down to `deviceId`.
   */
  readonly mhLink?: string;
}

/**
 * Renders an Android device ID with its Buganizer link, a link to the
 * device details page and a link to Mobile Harness. Shared by the Android
 * device list and the Android repair queue.
 */
export const AndroidDeviceIdCell = ({
  deviceId,
  workspace,
  mhLink,
}: AndroidDeviceIdCellProps) => {
  const internalLink = generateDeviceDetailsURL(
    workspaces[workspace].baseUrl,
    deviceId,
  );

  const href = mhLink || getMobileHarnessDeviceListLink(deviceId);

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        maxWidth: '100%',
      }}
    >
      <BuganizerLink name={deviceId} project="android" />
      <Link
        to={internalLink}
        style={{
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          minWidth: 0,
        }}
      >
        {deviceId}
      </Link>
      <Tooltip title="Open in Mobile Harness">
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          aria-label="Open in Mobile Harness"
          style={{ display: 'flex', alignItems: 'center' }}
        >
          <OpenInNewIcon fontSize="small" />
        </a>
      </Tooltip>
    </div>
  );
};
