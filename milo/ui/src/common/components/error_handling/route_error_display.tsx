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

import { Link } from '@mui/material';
import { useRouteError } from 'react-router';

import { ErrorDisplay } from './error_display';

export function RouteErrorDisplay() {
  const error = useRouteError() as Error;

  const isLazyLoadingAssetError = error.message.startsWith(
    'Failed to fetch dynamically imported module: ',
  );

  return (
    <ErrorDisplay
      error={error}
      instruction={
        // Do not refresh automatically to avoid an infinite refresh loop if
        // something goes wrong.
        isLazyLoadingAssetError ? (
          <b>
            You are likely on an outdated version of LUCI UI. Try{' '}
            <Link href={self.location.href}>reloading</Link> the page.
          </b>
        ) : null
      }
    />
  );
}
