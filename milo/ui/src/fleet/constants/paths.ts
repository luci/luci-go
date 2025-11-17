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
import {
  Platform,
  platformToJSON,
} from '../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

export const FLEET_CONSOLE_BASE_URL = '/ui/fleet/labs';

export const PLATFORM_SUBROUTE = 'p/:platform';
export const DEVICES_SUBROUTE = 'devices';
export const REPAIRS_SUBROUTE = 'repairs';
export const REQUESTS_SUBROUTE = 'requests';

export const PLATFORM_PATH = `${FLEET_CONSOLE_BASE_URL}/${PLATFORM_SUBROUTE}`;
export const DEVICE_LIST_PATH = `${PLATFORM_PATH}/${DEVICES_SUBROUTE}`;
export const DEVICE_DETAILS_PATH = `${PLATFORM_PATH}/${DEVICES_SUBROUTE}/:id`;
export const REPAIRS_PATH = `${PLATFORM_PATH}/${REPAIRS_SUBROUTE}`;

export const REQUESTS_PATH = `${FLEET_CONSOLE_BASE_URL}/${REQUESTS_SUBROUTE}`;

export const platformToURL = (p: Platform) => {
  return platformToJSON(p).toLowerCase();
};

export const CHROMEOS_PLATFORM = platformToURL(Platform.CHROMEOS);
export const ANDROID_PLATFORM = platformToURL(Platform.ANDROID);

export const generateDeviceListURL = (platform: string) => {
  return `${FLEET_CONSOLE_BASE_URL}/p/${platform}/${DEVICES_SUBROUTE}`;
};

export const generateRepairsURL = (platform: string) => {
  return `${FLEET_CONSOLE_BASE_URL}/p/${platform}/${REPAIRS_SUBROUTE}`;
};

export const generateDeviceDetailsURL = (platform: string, id: string) => {
  return `${FLEET_CONSOLE_BASE_URL}/p/${platform}/${DEVICES_SUBROUTE}/${id}`;
};

export const generateAnotherPlatformCorrespondingURL = (
  platform: Platform,
  newPlatform: Platform,
) => {
  return location.pathname.replace(
    'p/' + platformToURL(platform),
    'p/' + platformToURL(newPlatform),
  );
};

export const generateChromeOsDeviceDetailsURL = (id: string) => {
  return generateDeviceDetailsURL(CHROMEOS_PLATFORM, id);
};
