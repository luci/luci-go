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

type ValidPlatformReturn = {
  setPlatform: (p: Platform) => void;
  platform: Platform;
  isValid: true;
  currentPageSupportsPlatforms: true;
};
type InvalidPlatformReturn = Omit<
  ValidPlatformReturn,
  'platform' | 'isValid' | 'currentPageSupportsPlatforms'
> & {
  platform: undefined;
  isValid: false;
  currentPageSupportsPlatforms: boolean;
};

export function usePlatform(): ValidPlatformReturn | InvalidPlatformReturn {
  const { platform } = useParams();
  const location = useLocation();
  const navigate = useNavigate();

  const setPlatform = (newPlatform: Platform) => {
    if (!platform) throw Error('Platform not available in this page');

    const newPath = generatePath(
      location.pathname.replace(platform, ':platform'), // replaces the first occurrence
      { platform: platformToJSON(newPlatform).toLowerCase() },
    );
    navigate(newPath);
  };

  const platformEnum = getPlatformEnum(platform);
  if (platformEnum !== undefined) {
    return {
      platform: platformEnum,
      setPlatform,
      isValid: true,
      currentPageSupportsPlatforms: true,
    };
  }

  return {
    platform: undefined,
    setPlatform,
    isValid: false,
    currentPageSupportsPlatforms: platform !== undefined,
  };
}

const getPlatformEnum = (platform: string | undefined) => {
  if (!platform) {
    return undefined;
  }
  try {
    return platformFromJSON(platform.toUpperCase());
  } catch (e) {
    return undefined;
  }
};
