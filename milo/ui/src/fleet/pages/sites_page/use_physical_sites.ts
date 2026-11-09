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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';

import androidLogo from '@/fleet/assets/logos/android.png';
import browserLogo from '@/fleet/assets/logos/browser.png';
import chromeosLogo from '@/fleet/assets/logos/chromeos.png';
import { normalizeFilterKey } from '@/fleet/components/filters/normalize_filter_key';
import { StringListFilterCategoryBuilder } from '@/fleet/components/filters/string_list_filter';
import * as uf from '@/fleet/components/filters/use_filters';
import { BLANK_VALUE } from '@/fleet/constants/filters';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useBrowserDeviceDimensions } from '@/fleet/pages/device_list_page/browser/use_browser_device_dimensions';
import { useDeviceDimensions } from '@/fleet/pages/device_list_page/common/use_device_dimensions';
import { parseChromeosDeviceMetrics } from '@/fleet/utils/metrics';
import { combineAipFiltersWithParentheses } from '@/fleet/utils/search_param';
import { workspaces } from '@/fleet/workspaces';
import * as rpc from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import * as sh from './site_helpers';

export const PHYSICAL_SITES_QUERY_KEY = ['ListPhysicalSites'] as const;
const P = rpc.Platform;

export const PLATFORM_LOGOS: Partial<Record<rpc.Platform, string>> = {
  [P.CHROMEOS]: chromeosLogo,
  [P.ANDROID]: androidLogo,
  [P.CHROMIUM]: browserLogo,
};

const fDef = (key: string, label: string, csv: string) => ({
  key,
  label,
  values: csv.split(','),
});

const BASELINE_FILTERS: Partial<
  Record<rpc.Platform, readonly ReturnType<typeof fDef>[]>
> = {
  [P.CHROMEOS]: [
    fDef('labels."zone"', 'Zone', 'ZONE_SFO36_OS,ZONE_MTV96_OS,ZONE_IAD65_OS'),
    fDef('labels."pool"', 'Pool', 'DUT_POOL_QUOTA,wificell,chameleon_audio'),
    fDef('labels."hive"', 'Hive', 'e,f,chromeos1,chromeos6,chromeos8'),
    fDef('labels."board"', 'Board', 'brya,dedede,corsola,nissa,volteer'),
  ],
  [P.ANDROID]: [
    fDef(
      '"lab_name"',
      'Lab name',
      'acs,acs-1265,atc,sjc,sjc-mdpt9,sjc-mdpt9-adapt,sjc-mdpt9-wear,slaas-aep,us-mtv-tv-1667',
    ),
    fDef('"run_target"', 'Run target', 'orion,husky,shiba,komodo,caiman'),
    fDef('"pool"', 'Pool', 'default,wear,presubmit,postsubmit'),
    fDef('"host_group"', 'Host group', 'crystalball_performance,adapt,wear,xr'),
    fDef('"hostname"', 'Hostname', 'mtv946,sfo36,iad65,sjc-mdpt9,pao-em25'),
  ],
  [P.CHROMIUM]: [
    fDef('ufs."zone"', 'Zone (UFS)', 'SFO36_BROWSER,ZONE_MTV96,ZONE_IAD65'),
    fDef('ufs_zone', 'Zone', 'SFO36_BROWSER,ZONE_MTV96,ZONE_IAD65,ZONE_ATL97'),
    fDef('sw."pool"', 'Pool (Swarming)', 'chromium.tests,chromium.tests.gpu'),
    fDef('ufs."os"', 'OS (UFS)', 'Linux,Windows,Mac'),
  ],
};

export const usePhysicalSites = () => {
  const c = useFleetConsoleClient();
  const qc = useQueryClient();
  const q = useQuery({
    ...c.ListPhysicalSites.query({}),
    queryKey: PHYSICAL_SITES_QUERY_KEY,
    retry: false,
  });
  const sites = useMemo(() => q.data?.physicalSites ?? [], [q.data]);
  const onSuccess = () =>
    qc.invalidateQueries({ queryKey: PHYSICAL_SITES_QUERY_KEY });
  const mkReq = rpc.CreatePhysicalSiteRequest.fromPartial;
  const upReq = rpc.UpdatePhysicalSiteRequest.fromPartial;
  const rmReq = rpc.DeletePhysicalSiteRequest.fromPartial;
  const createSiteMutation = useMutation({
    mutationFn: (physicalSite: rpc.PhysicalSite) =>
      c.CreatePhysicalSite(mkReq({ physicalSite })),
    onSuccess,
  });
  const updateSiteMutation = useMutation({
    mutationFn: (physicalSite: rpc.PhysicalSite) =>
      c.UpdatePhysicalSite(upReq({ physicalSite })),
    onSuccess,
  });
  const deleteSiteMutation = useMutation({
    mutationFn: (id: string) => c.DeletePhysicalSite(rmReq({ id })),
    onSuccess,
  });
  return {
    sites,
    isLoading: q.isLoading,
    isError: q.isError,
    error: q.error,
    createSiteMutation,
    updateSiteMutation,
    deleteSiteMutation,
  };
};

type CountSt = Omit<sh.RawPlatformCountInput, 'platform' | 'filter'>;
const mkSt = (
  total: number | null,
  healthyCount: number | null,
  q: { isLoading: boolean; isError: boolean },
  isHealthTbd = false,
): CountSt => ({
  total,
  healthyCount,
  isHealthTbd,
  isLoading: q.isLoading,
  isError: q.isError,
});

export function useSitePlatformMetrics(
  platformFilters: readonly rpc.PlatformDeviceFilter[] | undefined,
): sh.SitePlatformRollupSummary {
  const c = useFleetConsoleClient();
  const findF = (p: rpc.Platform) =>
    platformFilters?.find((e) => e.platform === p)?.filter.trim() ?? '';
  const [crosF, androidF, browserF] = [
    findF(P.CHROMEOS),
    findF(P.ANDROID),
    findF(P.CHROMIUM),
  ];
  const crosQ = useQuery({
    ...c.CountDevices.query({ filter: crosF, platform: P.CHROMEOS }),
    enabled: Boolean(crosF),
  });
  const androidQueryFilter = androidF
    ? combineAipFiltersWithParentheses(workspaces.Android.baseFilter, androidF)
    : '';
  const androidQ = useQuery({
    ...c.CountAndroidDevices.query({ filter: androidQueryFilter }),
    enabled: Boolean(androidF),
  });
  const browserQ = useQuery({
    ...c.CountBrowserDevices.query({ filter: browserF }),
    enabled: Boolean(browserF),
  });
  const [cD, aD, bD] = [crosQ.data, androidQ.data, browserQ.data];
  const qSt = `${crosQ.isLoading}-${crosQ.isError}-${androidQ.isLoading}-${androidQ.isError}-${browserQ.isLoading}-${browserQ.isError}`;

  return useMemo(() => {
    const cTot = cD ? (cD.total ?? 0) : null;
    const cH =
      cD && cTot !== null
        ? parseChromeosDeviceMetrics(cD.deviceState, cTot).healthy
        : null;
    const byPlat: Partial<Record<rpc.Platform, CountSt>> = {
      [P.CHROMEOS]: mkSt(cTot, cH, crosQ),
      [P.ANDROID]: mkSt(
        aD ? (aD.totalDevices ?? 0) : null,
        aD ? (aD.healthCategoryInService?.total ?? 0) : null,
        androidQ,
      ),
      [P.CHROMIUM]: mkSt(bD ? (bD.total ?? 0) : null, null, browserQ, true),
    };
    return sh.computeSitePlatformRollup(
      (platformFilters ?? []).flatMap((e) => {
        const st = byPlat[e.platform];
        return e.filter.trim() && st
          ? [{ platform: e.platform, filter: e.filter.trim(), ...st }]
          : [];
      }),
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [aD, bD, cD, platformFilters, qSt]);
}

const mkBuilder = (label: string, values: readonly string[]) => {
  const uniq = [BLANK_VALUE, ...new Set(values.filter(Boolean))];
  return new StringListFilterCategoryBuilder()
    .setLabel(label)
    .setOptions(uniq.map((v) => ({ label: v, value: v })))
    .setIsLoading(true);
};

export function useSitePlatformFilterState(
  platform: rpc.Platform,
  filterValue: string,
  onChangeFilter: (nextAip160: string) => void,
): { filterCategoryDatas: uf.FilterCategory[]; isLoading: boolean } {
  const isDev = platform === P.CHROMEOS || platform === P.ANDROID;
  const devQ = useDeviceDimensions({ platform, enabled: isDev });
  const brwQ = useBrowserDeviceDimensions();

  const builders = useMemo(() => {
    const map: Record<string, StringListFilterCategoryBuilder> = {};
    const lists = new Map<string, { label: string; values: string[] }>();
    for (const d of BASELINE_FILTERS[platform] ?? []) {
      lists.set(d.key, { label: d.label, values: [...d.values] });
    }
    const addMap = (
      obj: Record<string, { values?: readonly string[] }> | undefined,
      fmtK: (k: string) => string,
      sfx = '',
    ) => {
      for (const [raw, v] of Object.entries(obj ?? {})) {
        const vals = v.values ?? [];
        if (!vals.length) continue;
        const [key, norm] = [fmtK(raw), normalizeFilterKey(raw).toLowerCase()];
        const hit = Array.from(lists.keys()).find(
          (k) => normalizeFilterKey(k).toLowerCase() === norm,
        );
        if (hit) lists.get(hit)!.values.push(...vals);
        else lists.set(key, { label: `${raw}${sfx}`, values: [...vals] });
      }
    };
    if (isDev && devQ.data) {
      addMap(devQ.data.baseDimensions, (k) => `"${k}"`);
      addMap(devQ.data.labels, (k) => `labels."${k}"`);
    } else if (platform === P.CHROMIUM && brwQ.data) {
      addMap(brwQ.data.baseDimensions, (k) => k);
      addMap(brwQ.data.ufsLabels, (k) => `ufs."${k}"`, ' (UFS)');
      addMap(brwQ.data.swarmingLabels, (k) => `sw."${k}"`, ' (Swarming)');
    }
    for (const [k, { label, values }] of lists.entries()) {
      map[k] = mkBuilder(label, values);
    }
    if (filterValue.trim()) {
      const parsed = uf.constructFiltersFromAIP160(filterValue);
      for (const rawKey of parsed.isError ? [] : Object.keys(parsed.terms)) {
        const norm = normalizeFilterKey(rawKey).toLowerCase();
        const exists = Object.keys(map).some(
          (k) => normalizeFilterKey(k).toLowerCase() === norm,
        );
        if (!exists) map[rawKey] = mkBuilder(normalizeFilterKey(rawKey), []);
      }
    }
    return map;
  }, [brwQ.data, devQ.data, filterValue, isDev, platform]);

  const isLoading = platform === P.CHROMIUM ? brwQ.isLoading : devQ.isLoading;
  const st = uf.useFilterState(builders, filterValue || null, onChangeFilter, {
    areFilterValuesLoading: isLoading,
  });
  const filterCategoryDatas = useMemo(
    () => (st.filterValues ? Object.values(st.filterValues) : []),
    [st.filterValues],
  );
  return { filterCategoryDatas, isLoading };
}
