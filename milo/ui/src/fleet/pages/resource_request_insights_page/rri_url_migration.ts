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

export type UrlMapper = (params: URLSearchParams) => URLSearchParams;

export const VERSION_PARAM_KEY = 'v';
export const LATEST_VERSION = 2;

// Version 1 to Version 2 mapper.
// Converts legacy RRI URL format (query string inside 'filters' param) to AIP-160.
export const v1_to_v2: UrlMapper = (
  params: URLSearchParams,
): URLSearchParams => {
  const newParams = new URLSearchParams(params);
  const filtersStr = params.get('filters');

  if (!filtersStr) {
    return newParams;
  }

  // The legacy format stored filters as a query string inside the 'filters' parameter.
  const filtersParams = new URLSearchParams(filtersStr);
  const aip160Parts: string[] = [];

  filtersParams.forEach((value, key) => {
    if (key.endsWith('_min')) {
      const baseKey = key.slice(0, -4);
      aip160Parts.push(`${baseKey} >= "${value}"`);
    } else if (key.endsWith('_max')) {
      const baseKey = key.slice(0, -4);
      aip160Parts.push(`${baseKey} <= "${value}"`);
    } else if (value.startsWith('(') && value.endsWith(')')) {
      const options = value
        .slice(1, -1)
        .split(' OR ')
        .map((opt) =>
          opt.startsWith('"') && opt.endsWith('"') ? opt.slice(1, -1) : opt,
        );
      const orPart = options.map((opt) => `${key} = "${opt}"`).join(' OR ');
      aip160Parts.push(`(${orPart})`);
    } else {
      const val =
        value.startsWith('"') && value.endsWith('"')
          ? value.slice(1, -1)
          : value;
      aip160Parts.push(`${key} = "${val}"`);
    }
  });

  if (aip160Parts.length > 0) {
    newParams.set('filters', aip160Parts.join(' AND '));
  }

  return newParams;
};

// Registry of mappers.
// Key is the source version.
export const MAPPERS: Record<number, UrlMapper> = {
  1: v1_to_v2,
};

/**
 * Detects the version of the URL state.
 * Defaults to 1 if the version parameter is missing.
 */
export const detectVersion = (params: URLSearchParams): number => {
  const vStr = params.get(VERSION_PARAM_KEY);
  if (!vStr) {
    return 1;
  }
  const v = parseInt(vStr, 10);
  return isNaN(v) ? 1 : v;
};

/**
 * Maps the URL search parameters transitively from the detected version to the latest version.
 * Returns the migrated parameters and a boolean indicating if any migration occurred.
 */
export const mapUrl = (
  params: URLSearchParams,
): { migratedParams: URLSearchParams; needsMigration: boolean } => {
  let currentVersion = detectVersion(params);
  let currentParams = new URLSearchParams(params);
  let needsMigration = false;

  while (currentVersion < LATEST_VERSION) {
    const mapper = MAPPERS[currentVersion];
    if (!mapper) {
      // eslint-disable-next-line no-console
      console.warn(`No mapper found for version ${currentVersion}`);
      break;
    }
    currentParams = mapper(currentParams);
    currentVersion++;
    needsMigration = true;
  }

  if (needsMigration) {
    currentParams.set(VERSION_PARAM_KEY, LATEST_VERSION.toString());
  }

  return { migratedParams: currentParams, needsMigration };
};
