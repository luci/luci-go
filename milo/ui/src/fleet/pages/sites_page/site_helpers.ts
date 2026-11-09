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

import * as paths from '@/fleet/constants/paths';
import {
  Platform,
  PlatformDeviceFilter,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

export interface SitePlatformConfig {
  readonly platform: Platform;
  readonly label: string;
  readonly slug: string;
  readonly placeholder: string;
}

const P = Platform;
const mkCfg = (
  platform: Platform,
  label: string,
  slug: string,
  hint: string,
): SitePlatformConfig => ({
  platform,
  label,
  slug,
  placeholder: `Filter ${label} devices (${hint})...`,
});

export const SUPPORTED_SITE_PLATFORMS: readonly SitePlatformConfig[] = [
  mkCfg(P.CHROMEOS, 'ChromeOS', paths.CHROMEOS_PLATFORM, 'e.g. zone, pool'),
  mkCfg(P.ANDROID, 'Android', paths.ANDROID_PLATFORM, 'e.g. lab_name, pool'),
  mkCfg(P.CHROMIUM, 'Browser', paths.CHROMIUM_PLATFORM, 'e.g. zone, pool, os'),
];

export interface RawPlatformCountInput {
  readonly platform: Platform;
  readonly filter: string;
  readonly total: number | null;
  readonly healthyCount: number | null;
  readonly isHealthTbd?: boolean;
  readonly isLoading: boolean;
  readonly isError: boolean;
}

export function getCampusMapsUrl(siteId: string): string | null {
  const s = siteId.trim();
  if (!s) return null;
  const p = s.split('-').filter(Boolean);
  const bldg = p.length >= 3 ? p.slice(0, 3).join('-') : s;
  return `https://goto.google.com/where/?id=${encodeURIComponent(bldg)}`;
}

export function formatSiteUpdatedAt(updatedAt?: string): string | null {
  const d = updatedAt ? new Date(updatedAt) : null;
  return d && !Number.isNaN(d.getTime()) ? d.toISOString().slice(0, 10) : null;
}

export function getPlatformLabel(platform: Platform): string {
  const c = SUPPORTED_SITE_PLATFORMS.find((p) => p.platform === platform);
  return c ? c.label : platform === P.PIXEL ? 'Pixel' : 'Unknown';
}

export function getPlatformDevicesUrl(platform: Platform, filter: string) {
  const f = filter.trim();
  const c = SUPPORTED_SITE_PLATFORMS.find((p) => p.platform === platform);
  if (!f || !c) return null;
  return `${paths.generateDeviceListURL(c.slug)}?${new URLSearchParams({ filters: f })}`;
}

export function formatHealthyPercentage(pct: number): string {
  return `${(Math.round(Math.min(Math.max(pct, 0), 100) * 10) / 10).toFixed(1)}%`;
}

export function computeSitePlatformRollup(
  inputs: readonly RawPlatformCountInput[],
) {
  const valid = inputs.flatMap((i) => {
    const url = getPlatformDevicesUrl(i.platform, i.filter);
    return url ? [{ ...i, filter: i.filter.trim(), url }] : [];
  });
  const loaded = valid.filter((r) => r.total !== null);
  const tot = loaded.length
    ? loaded.reduce((s, r) => s + (r.total ?? 0), 0)
    : null;
  const hRows = valid.filter(
    (i) => !i.isHealthTbd && i.total && i.healthyCount !== null,
  );
  const hDen = hRows.reduce((s, r) => s + r.total!, 0);
  const hNum = hRows.reduce(
    (s, r) => s + Math.min(r.healthyCount!, r.total!),
    0,
  );
  const pct = (n: number, d: number) => Math.min((n / d) * 100, 100);
  return {
    rows: valid.map((i) => {
      const hasS = valid.length >= 2 && tot && i.total !== null;
      const hasH = !i.isHealthTbd && i.total && i.healthyCount !== null;
      return {
        ...i,
        label: getPlatformLabel(i.platform),
        sharePercent: hasS ? Math.round((i.total! / tot!) * 100) : null,
        healthyPercent: hasH ? pct(i.healthyCount!, i.total!) : null,
        isHealthTbd: Boolean(i.isHealthTbd),
      };
    }),
    totalDevices: tot,
    overallHealthyPercent: hDen > 0 ? pct(hNum, hDen) : null,
    isAllHealthTbd: valid.length > 0 && valid.every((r) => r.isHealthTbd),
    isAnyLoading: valid.some((r) => r.isLoading),
    isAnyError: valid.some((r) => r.isError),
  };
}

export type SitePlatformRollupSummary = ReturnType<
  typeof computeSitePlatformRollup
>;

export function platformFiltersToMap(
  filters: readonly PlatformDeviceFilter[] | undefined,
) {
  const map: Partial<Record<Platform, string>> = {};
  for (const e of filters ?? []) {
    if (e.filter.trim() && map[e.platform] === undefined) {
      map[e.platform] = e.filter.trim();
    }
  }
  return map;
}

export function buildPlatformFilters(
  filterByPlatform: Partial<Record<Platform, string>>,
) {
  return SUPPORTED_SITE_PLATFORMS.flatMap(({ platform }) => {
    const filter = filterByPlatform[platform]?.trim() ?? '';
    return filter
      ? [PlatformDeviceFilter.fromPartial({ platform, filter })]
      : [];
  });
}
