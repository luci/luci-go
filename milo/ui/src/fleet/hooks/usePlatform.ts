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

import _ from 'lodash';
import {
  generatePath,
  useParams,
  useLocation,
  useNavigate,
} from 'react-router';

import {
  Platform,
  platformFromJSON,
  platformToJSON,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export type PlatformDetails = {
  setPlatform: (p: Platform) => void;
  platform?: Platform;
  inPlatformScope: boolean;
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
  const { platform } = useParams();
  const location = useLocation();
  const navigate = useNavigate();

  // Assume any page which has a platform route param is in the platform scope.
  const inPlatformScope = !!platform;

  const setPlatform = (newPlatform: Platform) => {
    if (!inPlatformScope) throw Error('Platform not available in this page');

    const newPath = generatePath(
      location.pathname.replace(platform, ':platform'), // replaces the first occurrence
      { platform: platformToJSON(newPlatform).toLowerCase() },
    );
    navigate(newPath);
  };

  return {
    platform: getPlatformEnum(platform),
    setPlatform,
    inPlatformScope,
  };
}

const getPlatformEnum = (platform: string | undefined) => {
  if (!platform) return undefined;
  try {
    return platformFromJSON(platform.toUpperCase());
  } catch (_) {
    return undefined;
  }
};
