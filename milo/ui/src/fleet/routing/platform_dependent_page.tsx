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

import { isValidElement } from 'react';

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { PlatformNotAvailable } from '../components/platform_not_available';
import { usePlatform } from '../hooks/usePlatform';
import { PageNotFoundPage } from '../pages/not_found_page';

import { PageComponentMap } from './platform_routes';

interface PlatformDependentPageProps {
  pageComponentMap: PageComponentMap;
}

export const PlatformDependentPage = ({
  pageComponentMap,
}: PlatformDependentPageProps) => {
  const { platform } = usePlatform();

  const Component = pageComponentMap[platform];

  if (Component) {
    if (isValidElement(Component)) {
      return Component;
    }

    return <Component />;
  }

  const availablePlatforms = Object.keys(pageComponentMap).map(
    (p) => Number(p) as Platform,
  );
  if (availablePlatforms.length > 0) {
    return <PlatformNotAvailable availablePlatforms={availablePlatforms} />;
  }

  return <PageNotFoundPage />;
};
