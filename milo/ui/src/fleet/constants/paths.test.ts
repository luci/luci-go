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

import { Platform } from '../../proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  generateAnotherPlatformCorrespondingURL,
  platformToURL,
} from './paths';

describe('generateAnotherPlatformCorrespondingURL', () => {
  const setUrl = (url: string) => {
    window.history.replaceState(null, '', url);
  };

  it('routes to device list when on a devices route', () => {
    setUrl('/ui/fleet/p/chromeos/devices');
    const result = generateAnotherPlatformCorrespondingURL(
      Platform.CHROMEOS,
      Platform.ANDROID,
    );
    expect(result).toBe('/ui/fleet/p/android/devices');
  });

  it('routes to repairs workforce when switching to ChromeOS on workforce route', () => {
    setUrl('/ui/fleet/p/android/repairs/workforce');
    const result = generateAnotherPlatformCorrespondingURL(
      Platform.ANDROID,
      Platform.CHROMEOS,
    );
    expect(result).toBe('/ui/fleet/p/chromeos/repairs/workforce');
  });

  it('routes to standard repairs when on repairs route without workforce', () => {
    setUrl('/ui/fleet/p/chromeos/repairs');
    const result = generateAnotherPlatformCorrespondingURL(
      Platform.CHROMEOS,
      Platform.ANDROID,
    );
    expect(result).toBe('/ui/fleet/p/android/repairs');
  });

  it('routes to admin-tasks for ChromeOS and devices list for non-ChromeOS', () => {
    setUrl('/ui/fleet/p/android/admin-tasks');
    const crosResult = generateAnotherPlatformCorrespondingURL(
      Platform.ANDROID,
      Platform.CHROMEOS,
    );
    expect(crosResult).toBe('/ui/fleet/p/chromeos/admin-tasks');

    setUrl('/ui/fleet/p/chromeos/admin-tasks');
    const androidResult = generateAnotherPlatformCorrespondingURL(
      Platform.CHROMEOS,
      Platform.ANDROID,
    );
    expect(androidResult).toBe('/ui/fleet/p/android/devices');
  });

  it('handles subroutes with query parameters correctly', () => {
    setUrl('/ui/fleet/p/chromeos/admin-tasks?tab=recent');
    const androidResult = generateAnotherPlatformCorrespondingURL(
      Platform.CHROMEOS,
      Platform.ANDROID,
    );
    expect(androidResult).toBe('/ui/fleet/p/android/devices');
  });

  it('routes to health for ChromeOS and devices list for non-ChromeOS', () => {
    setUrl('/ui/fleet/p/android/health');
    const crosResult = generateAnotherPlatformCorrespondingURL(
      Platform.ANDROID,
      Platform.CHROMEOS,
    );
    expect(crosResult).toBe('/ui/fleet/p/chromeos/health');

    setUrl('/ui/fleet/p/chromeos/health');
    const androidResult = generateAnotherPlatformCorrespondingURL(
      Platform.CHROMEOS,
      Platform.ANDROID,
    );
    expect(androidResult).toBe('/ui/fleet/p/android/devices');
  });

  it('falls back to replacing platform in pathname for unknown subroutes', () => {
    setUrl('/ui/fleet/p/chromeos/custom-view');
    const result = generateAnotherPlatformCorrespondingURL(
      Platform.CHROMEOS,
      Platform.ANDROID,
    );
    expect(result).toBe('/ui/fleet/p/android/custom-view');
  });

  it('platformToURL converts platform correctly', () => {
    expect(platformToURL(Platform.CHROMEOS)).toBe('chromeos');
    expect(platformToURL(Platform.ANDROID)).toBe('android');
    expect(platformToURL(Platform.CHROMIUM)).toBe('chromium');
    expect(platformToURL(Platform.PIXEL)).toBe('pixel');
  });
});
