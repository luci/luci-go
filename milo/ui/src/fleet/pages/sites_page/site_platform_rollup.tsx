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

import { Box, Button, Link, Tooltip, Typography } from '@mui/material';
import { useMemo } from 'react';
import { Link as RouterLink } from 'react-router';

import { FilterBar } from '@/fleet/components/filter_dropdown/filter_bar';
import { actionRowSx } from '@/fleet/theme/recipes';
import * as tk from '@/fleet/theme/tokens';
import {
  Platform,
  PlatformDeviceFilter,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import * as sh from './site_helpers';
import * as ps from './use_physical_sites';

type RollupRow = sh.SitePlatformRollupSummary['rows'][number];
export type HealthStateLike = Partial<sh.SitePlatformRollupSummary & RollupRow>;

const c = tk.fleetColors;
const bdr = tk.borders.divider;
const fs = tk.fontSizes.caption;
const tnumSx = { fontVariantNumeric: 'tabular-nums' } as const;
const rightSx = {
  textAlign: 'right',
  whiteSpace: 'nowrap',
  ...tnumSx,
} as const;
const betweenSx = {
  ...actionRowSx,
  flexWrap: 'nowrap',
  justifyContent: 'space-between',
} as const;
const inlineSx = {
  ...actionRowSx,
  flexWrap: 'nowrap',
  gap: 0.75,
  minWidth: 0,
} as const;
const linkSx = { fontSize: fs, fontWeight: 500, ...tnumSx } as const;
const colSx = { display: 'flex', flexDirection: 'column', gap: 0.75 } as const;
const logoSx = {
  width: 16,
  height: 16,
  objectFit: 'contain',
  flexShrink: 0,
} as const;
const bdrSx = { border: bdr, borderRadius: tk.radii.sm } as const;
const cleanRowSx = {
  display: 'grid',
  gridTemplateColumns: 'minmax(0, 1fr) auto 68px',
  alignItems: 'center',
  columnGap: 1.5,
  minHeight: 30,
  py: 0.375,
} as const;
const emptyRowSx = {
  ...betweenSx,
  ...bdrSx,
  bgcolor: c.surfaceTonal,
  px: 1.5,
  py: 1,
};
const filterCardSx = {
  ...colSx,
  ...bdrSx,
  bgcolor: c.surfaceTonal,
  p: 1.25,
  gap: 1,
} as const;

export const HealthIndicator = ({ state }: { state: HealthStateLike }) => {
  const pct = state.healthyPercent ?? state.overallHealthyPercent ?? null;
  const isTbd = Boolean(state.isHealthTbd ?? state.isAllHealthTbd);
  const isLoading = Boolean(state.isLoading ?? state.isAnyLoading);
  const isError = Boolean(state.isError ?? state.isAnyError);
  const st = tk.statusTones;
  if (isError || isLoading || (pct === null && !isTbd)) {
    const color = isError ? st.error.fg : c.textSecondary;
    const txt = isError ? 'Error' : isLoading ? '...' : '—';
    return (
      <Box component="span" sx={{ fontSize: fs, color }}>
        {txt}
      </Box>
    );
  }
  if (isTbd) {
    return (
      <Tooltip title="Browser readiness metric is TBD">
        <Box
          component="span"
          tabIndex={0}
          sx={{ fontSize: fs, color: c.textTertiary }}
        >
          TBD
        </Box>
      </Tooltip>
    );
  }
  const t = pct! >= 85 ? st.success : pct! >= 70 ? st.warning : st.error;
  const sx = {
    ...inlineSx,
    justifyContent: 'flex-end',
    gap: 0.5,
    ...linkSx,
    color: t.fg,
  };
  const dotSx = {
    width: 6,
    height: 6,
    borderRadius: tk.radii.pill,
    bgcolor: t.dot,
  };
  return (
    <Box component="span" sx={sx}>
      <Box component="span" sx={dotSx} />
      {sh.formatHealthyPercentage(pct!)}
    </Box>
  );
};

export interface SitePlatformRollupProps {
  readonly platformFilters: readonly PlatformDeviceFilter[] | undefined;
  readonly onAddFilters?: () => void;
}

export const SitePlatformRollup = (p: SitePlatformRollupProps) => {
  const { platformFilters, onAddFilters } = p;
  const r = ps.useSitePlatformMetrics(platformFilters);
  if (r.rows.length === 0) {
    if (!onAddFilters) return null;
    return (
      <Box sx={emptyRowSx}>
        <Typography variant="caption" color="text.secondary" noWrap>
          No device filters configured
        </Typography>
        <Button size="small" variant="toolbar" onClick={onAddFilters}>
          Add filters
        </Button>
      </Box>
    );
  }
  const totCnt = r.isAnyLoading
    ? '...'
    : (r.totalDevices?.toLocaleString() ?? '—');

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
      {r.rows.map((row, idx) => {
        const logo = ps.PLATFORM_LOGOS[row.platform];
        const cnt = row.isLoading ? '...' : (row.total ?? 0).toLocaleString();
        const lbl = `${row.label} devices`;
        return (
          <Box
            key={`${row.platform}-${row.filter}`}
            data-testid={
              r.rows.length === 1 && idx === 0
                ? 'site-platform-rollup-total'
                : undefined
            }
            sx={{
              ...cleanRowSx,
              ...(idx > 0 ? { borderTop: bdr } : {}),
            }}
          >
            <Box sx={inlineSx}>
              {logo && <Box component="img" src={logo} alt="" sx={logoSx} />}
              <Typography variant="caption" noWrap fontWeight={500}>
                {row.label}
              </Typography>
            </Box>
            <Box sx={rightSx}>
              <Tooltip title={`View ${lbl}`}>
                <Link
                  component={RouterLink}
                  to={row.url}
                  aria-label={lbl}
                  underline="hover"
                  sx={linkSx}
                >
                  <Box component="span">{cnt}</Box>
                  {!row.isLoading && (
                    <Box component="span" sx={{ fontWeight: 400 }}>
                      {' devices'}
                    </Box>
                  )}
                </Link>
              </Tooltip>
            </Box>
            <Box sx={rightSx}>
              <HealthIndicator state={row} />
            </Box>
          </Box>
        );
      })}
      {r.rows.length > 1 && (
        <Box
          data-testid="site-platform-rollup-total"
          sx={{ ...cleanRowSx, borderTop: bdr, pt: 0.625, mt: 0.25 }}
        >
          <Typography variant="caption" fontWeight={600} noWrap>
            Total
          </Typography>
          <Typography variant="caption" fontWeight={600} sx={rightSx}>
            <Box component="span">{totCnt}</Box>
            {!r.isAnyLoading && r.totalDevices !== null && (
              <Box
                component="span"
                sx={{ color: c.textSecondary, fontWeight: 400 }}
              >
                {' devices'}
              </Box>
            )}
          </Typography>
          <Box sx={rightSx}>
            <HealthIndicator state={r} />
          </Box>
        </Box>
      )}
    </Box>
  );
};

interface FilterBarRowProps {
  readonly cfg: sh.SitePlatformConfig;
  readonly val: string;
  readonly row?: RollupRow;
  readonly onChange: (platform: Platform, value: string) => void;
}

const SitePlatformFilterBarRow = (p: FilterBarRowProps) => {
  const { cfg, val, row, onChange } = p;
  const fSt = ps.useSitePlatformFilterState(cfg.platform, val, (n) =>
    onChange(cfg.platform, n),
  );
  const logo = ps.PLATFORM_LOGOS[cfg.platform];
  const cntTxt = row?.isLoading
    ? '...'
    : `${(row?.total ?? 0).toLocaleString()} devices`;

  return (
    <Box data-testid={`site-filter-bar-${cfg.slug}`} sx={filterCardSx}>
      <Box sx={betweenSx}>
        <Box sx={inlineSx}>
          {logo && <Box component="img" src={logo} alt="" sx={logoSx} />}
          <Typography variant="caption" fontWeight={600}>
            {cfg.label}
          </Typography>
        </Box>
        {row ? (
          <Box sx={inlineSx}>
            <Link component={RouterLink} to={row.url} sx={linkSx}>
              {cntTxt}
            </Link>
            <HealthIndicator state={row} />
          </Box>
        ) : (
          <Typography variant="caption" color="text.secondary">
            No filter set
          </Typography>
        )}
      </Box>
      <FilterBar
        filterCategoryDatas={fSt.filterCategoryDatas}
        isLoading={fSt.isLoading}
        searchPlaceholder={cfg.placeholder}
        disableShortcut
      />
    </Box>
  );
};

export interface SitePlatformFilterEditorProps {
  readonly filterByPlatform: Partial<Record<Platform, string>>;
  readonly onChangeFilter: (platform: Platform, value: string) => void;
}

export const SitePlatformFilterEditor = (p: SitePlatformFilterEditorProps) => {
  const { filterByPlatform, onChangeFilter } = p;
  const draft = useMemo(
    () => sh.buildPlatformFilters(filterByPlatform),
    [filterByPlatform],
  );
  const r = ps.useSitePlatformMetrics(draft);
  const totTxt = r.isAnyLoading
    ? '...'
    : `${(r.totalDevices ?? 0).toLocaleString()} devices`;

  return (
    <Box sx={{ ...colSx, gap: 1.25 }}>
      <Box sx={betweenSx}>
        <Typography variant="subtitle2">Device filters by platform</Typography>
        {r.rows.length > 0 && (
          <Box sx={inlineSx}>
            <Typography variant="caption" fontWeight={600} sx={tnumSx}>
              {`Total: ${totTxt}`}
            </Typography>
            <HealthIndicator state={r} />
          </Box>
        )}
      </Box>
      {sh.SUPPORTED_SITE_PLATFORMS.map((c) => (
        <SitePlatformFilterBarRow
          key={c.platform}
          cfg={c}
          val={filterByPlatform[c.platform] ?? ''}
          row={r.rows.find((x) => x.platform === c.platform)}
          onChange={onChangeFilter}
        />
      ))}
    </Box>
  );
};
