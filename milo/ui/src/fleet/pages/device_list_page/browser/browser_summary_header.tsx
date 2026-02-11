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

import { PagerContext } from '@/common/components/params_pager';
import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { BROWSER_SWARMING_SOURCE } from '@/fleet/constants/browser';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import { getFilterQueryString } from '@/fleet/utils/search_param';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

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

export interface BrowserSummaryHeaderProps {
  selectedOptions: SelectedOptions;
  pagerContext: PagerContext;
}

export function BrowserSummaryHeader({
  selectedOptions,
  pagerContext,
}: BrowserSummaryHeaderProps) {
  const client = useFleetConsoleClient();
  const [searchParams] = useSyncedSearchParams();

  const countQuery = useQuery(
    client.CountBrowserDevices.query({
      filter: stringifyFilters(selectedOptions),
    }),
  );

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
              filterUrl={getFilterQueryString(
                {
                  [`${BROWSER_SWARMING_SOURCE}."state"`]: ['alive'],
                },
                searchParams,
                pagerContext,
              )}
            />
            <SingleMetric
              name="Dead"
              value={countQuery.data?.swarmingState?.dead}
              total={countQuery.data?.swarmingState?.total}
              loading={countQuery.isPending}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              filterUrl={getFilterQueryString(
                {
                  [`${BROWSER_SWARMING_SOURCE}."state"`]: ['dead'],
                },
                searchParams,
                pagerContext,
              )}
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
              filterUrl={getFilterQueryString(
                {
                  [`${BROWSER_SWARMING_SOURCE}."state"`]: ['quarantined'],
                },
                searchParams,
                pagerContext,
              )}
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
              filterUrl={getFilterQueryString(
                {
                  [`${BROWSER_SWARMING_SOURCE}."state"`]: ['maintenance'],
                },
                searchParams,
                pagerContext,
              )}
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
