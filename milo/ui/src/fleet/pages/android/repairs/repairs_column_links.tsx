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
import { Link } from 'react-router';

import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';

interface ExploreLinkProps {
  to: string;
  totalDevices: number;
}

export const ExploreInArsenalLink = ({
  to,
  totalDevices,
}: ExploreLinkProps) => {
  const { trackEvent } = useGoogleAnalytics();

  return (
    <Link
      aria-label="Explore in Arsenal"
      css={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100%',
      }}
      to={to}
      target="_blank"
      rel="noopener noreferrer"
      onClick={() =>
        trackEvent('explore_in_arsenal_clicked', {
          totalDevices: totalDevices,
        })
      }
    >
      <OpenInNewIcon />
    </Link>
  );
};

export const ExploreDevicesLink = ({ to, totalDevices }: ExploreLinkProps) => {
  const { trackEvent } = useGoogleAnalytics();

  return (
    <Link
      aria-label="Explore devices"
      css={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100%',
      }}
      to={to}
      target="_blank"
      rel="noopener noreferrer"
      onClick={() =>
        trackEvent('explore_devices_clicked', {
          totalDevices: totalDevices,
        })
      }
    >
      <OpenInNewIcon />
    </Link>
  );
};
