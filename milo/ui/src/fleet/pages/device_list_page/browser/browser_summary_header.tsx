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

import { type CSSObject } from '@emotion/styled';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import { StringListFilterCategory } from '@/fleet/components/filters/string_list_filter';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { BROWSER_SWARMING_SOURCE } from '@/fleet/constants/browser';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';

import { useBrowserFilters } from './use_browser_filters';

const HAS_RIGHT_SIBLING_STYLES: CSSObject = {
  borderRight: `1px solid ${colors.grey[300]}`,
  marginRight: 16,
  paddingRight: 8,
};

const METRIC_CONTAINER_STYLES: CSSObject = {
  display: 'flex',
  justifyContent: 'flex-start',
  marginTop: 4,
  flexWrap: 'wrap',
  gap: 8,
};

export function BrowserSummaryHeader() {
  const client = useFleetConsoleClient();
  const { filterValues, aip160 } = useBrowserFilters();

  const countQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: aip160(),
    }),
  );

  const setFilterOptions = (key: string, options: string[]) => {
    const filter = filterValues?.[key];
    if (filter instanceof StringListFilterCategory) {
      filter.setSelectedOptions(options);
    }
  };

  const getContent = () => {
    if (countQuery.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(countQuery.error, 'get the main metrics')}
        </Alert>
      );
    }

    return (
      <div css={{ display: 'flex', maxWidth: 1400 }}>
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
          }}
        >
          <Typography variant="subhead1">Total</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Devices"
              value={countQuery.data?.total}
              loading={countQuery.isPending}
            />
            <SingleMetric
              name="Bots"
              value={countQuery.data?.swarmingState?.total}
              loading={countQuery.isPending}
            />
          </div>
        </div>
        <div>
          <Typography variant="subhead1">Bot health</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Alive"
              value={countQuery.data?.swarmingState?.alive}
              total={countQuery.data?.swarmingState?.total}
              loading={countQuery.isPending}
              handleClick={() =>
                setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, [
                  'alive',
                ])
              }
            />
            <SingleMetric
              name="Dead"
              value={countQuery.data?.swarmingState?.dead}
              total={countQuery.data?.swarmingState?.total}
              loading={countQuery.isPending}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              handleClick={() =>
                setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, ['dead'])
              }
            />
            <SingleMetric
              name="Quarantined"
              value={countQuery.data?.swarmingState?.quarantined}
              total={countQuery.data?.swarmingState?.total}
              loading={countQuery.isPending}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
              handleClick={() =>
                setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, [
                  'quarantined',
                ])
              }
            />
            <SingleMetric
              name="Maintenance"
              value={countQuery.data?.swarmingState?.maintenance}
              total={countQuery.data?.swarmingState?.total}
              loading={countQuery.isPending}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
              handleClick={() =>
                setFilterOptions(`${BROWSER_SWARMING_SOURCE}."state"`, [
                  'maintenance',
                ])
              }
            />
          </div>
        </div>
      </div>
    );
  };

  return (
    <MetricsContainer>
      <Typography variant="h4">Main metrics</Typography>
      <div css={{ marginTop: 24 }}>{getContent()}</div>
    </MetricsContainer>
  );
}
