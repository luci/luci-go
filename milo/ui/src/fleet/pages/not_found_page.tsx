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

import { Typography } from '@mui/material';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import sadBass from '@/fleet/assets/pngs/sad_bass.png';
import { TrackLeafRoutePageView } from '@/generic_libs/components/google_analytics';

import { FleetHelmet } from '../layouts/fleet_helmet';

const PageNotFoundPage = () => {
  return (
    <div
      css={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexDirection: 'column',
        margin: 50,
      }}
    >
      <Typography variant="h1" sx={{ fontSize: 50, letterSpacing: '10.5px' }}>
        404
      </Typography>
      <img
        alt="logo"
        id="luci-icon"
        src={sadBass}
        css={{
          width: '50%',
          maxWidth: 400,
        }}
      />
      <Typography variant="h1" sx={{ fontSize: 50 }}>
        Page not found
      </Typography>
    </div>
  );
};

export function Component() {
  return (
    <TrackLeafRoutePageView contentGroup="fleet-console-404">
      <FleetHelmet pageTitle="Page not found" />
      <RecoverableErrorBoundary
        // See the documentation for `<LoginPage />` for why we handle error
        // this way.
        key="fleet-page-not-found"
      >
        <PageNotFoundPage />
      </RecoverableErrorBoundary>
    </TrackLeafRoutePageView>
  );
}
