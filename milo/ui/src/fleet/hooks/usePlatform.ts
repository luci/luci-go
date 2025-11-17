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

import { useNavigate, useParams } from 'react-router';

import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { generateAnotherPlatformCorrespondingURL } from '../constants/paths';

export type PlatformDetails = {
  setPlatform: (p: Platform) => void;
  platform: Platform;
};

export const platformRenderString = (p?: Platform) => {
  switch (p) {
    case Platform.ANDROID:
      return 'Android';
    case Platform.CHROMEOS:
      return 'Chrome OS';
    case Platform.CHROMIUM:
      return 'Chromium';
    case Platform.UNSPECIFIED:
      return 'Unspecified';
    case undefined:
      return undefined;
  }
};

export function usePlatform(): PlatformDetails {
  const platform = useCurrentPlatform();
  const navigate = useNavigate();

  // Assume any page which has a platform route param is in the platform scope.
  if (platform === undefined) {
    throw Error('Platform not available in this page');
  }

  const setPlatform = (newPlatform: Platform) => {
    navigate(generateAnotherPlatformCorrespondingURL(platform, newPlatform));
  };

  return {
    platform: platform,
    setPlatform,
  };
}

export function useCurrentPlatform() {
  const { platform } = useParams();
  if (platform === undefined) {
    return undefined;
  }

  switch (platform) {
    case 'android':
      return Platform.ANDROID;
    case 'chromeos':
      return Platform.CHROMEOS;
    case 'chromium':
      return Platform.CHROMIUM;
    case 'unspecified':
      return Platform.UNSPECIFIED;
    default:
      return Platform.UNSPECIFIED;
  }
}

export function useIsInPlatformScope() {
  const platform = useCurrentPlatform();

  return platform !== undefined && platform !== Platform.UNSPECIFIED;
}
