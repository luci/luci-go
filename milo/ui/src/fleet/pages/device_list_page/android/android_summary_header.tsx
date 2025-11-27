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

import { type CSSObject } from '@emotion/styled';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';

import {
  emptyPageTokenUpdater,
  PagerContext,
} from '@/common/components/params_pager';
import { stringifyFilters } from '@/fleet/components/filter_dropdown/parser/parser';
import { addNewFilterToParams } from '@/fleet/components/filter_dropdown/search_param_utils';
import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { colors } from '@/fleet/theme/colors';
import { SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { Platform } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

const METRIC_CONTAINER_STYLES: CSSObject = {
  display: 'flex',
  justifyContent: 'flex-start',
  marginTop: 4,
  flexWrap: 'wrap',
  gap: 8,
};

export interface AndroidSummaryHeaderProps {
  selectedOptions: SelectedOptions;
  pagerContext: PagerContext;
}

export function AndroidSummaryHeader({
  selectedOptions,
  pagerContext,
}: AndroidSummaryHeaderProps) {
  const client = useFleetConsoleClient();

  const countQuery = useQuery(
    client.CountDevices.query({
      filter: stringifyFilters(selectedOptions),
      platform: Platform.ANDROID,
    }),
  );
  const [searchParams, _] = useSyncedSearchParams();

  const getFilterQueryString = (filters: Record<string, string[]>): string => {
    let newSearchParams = new URLSearchParams(searchParams);
    newSearchParams = emptyPageTokenUpdater(pagerContext)(newSearchParams);
    for (const [name, values] of Object.entries(filters)) {
      newSearchParams = addNewFilterToParams(newSearchParams, name, values);
    }
    return '?' + newSearchParams.toString();
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
      <div
        css={{
          display: 'flex',
          maxWidth: 1400,
        }}
      >
        <div css={{ flexGrow: 2 }}>
          <Typography variant="subhead1">Device state</Typography>
          <div css={METRIC_CONTAINER_STYLES}>
            <SingleMetric
              name="Total"
              value={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
            />
            <SingleMetric
              name="Ready"
              value={countQuery.data?.androidCount?.idleDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['IDLE'],
              })}
            />
            <SingleMetric
              name="Busy"
              value={countQuery.data?.androidCount?.busyDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['BUSY'],
              })}
            />
            <SingleMetric
              name="Prepping"
              value={countQuery.data?.androidCount?.preppingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['PREPPING'],
              })}
            />
            <SingleMetric
              name="Missing"
              value={countQuery.data?.androidCount?.missingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['MISSING'],
              })}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
            />
            <SingleMetric
              name="Dying"
              value={countQuery.data?.androidCount?.dyingDevices}
              total={countQuery.data?.androidCount?.totalDevices}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString({
                state: ['DYING'],
              })}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
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
