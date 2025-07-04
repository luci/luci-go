// Copyright 2024 The LUCI Authors.
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

import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Typography } from '@mui/material';
import { UseQueryResult } from '@tanstack/react-query';

import { LeaseStateInfo } from '@/fleet/components/lease_state_info/lease_state_info';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { colors } from '@/fleet/theme/colors';
import { getErrorMessage } from '@/fleet/utils/errors';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { CountDevicesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { addNewFilterToParams } from '../../components/filter_dropdown/search_param_utils/search_param_utils';
import { SingleMetric } from '../../components/summary_header/single_metric';

const HAS_RIGHT_SIBLING_STYLES = {
  borderRight: `1px solid ${colors.grey[300]}`,
  marginRight: 15,
  paddingRight: 10,
};

interface MainMetricsProps {
  countQuery: UseQueryResult<CountDevicesResponse, unknown>;
}

// TODO: b/393624377 - Refactor this component to make it easier to test.
export function MainMetrics({ countQuery }: MainMetricsProps) {
  const [searchParams, _] = useSyncedSearchParams();

  /**
   * @param filterName name of the filter, ex. state
   * @param filterValue array of filter values, ex. DEVICE_STATE_LEASED
   * @returns will return URL query part with filters and existing parameters, like sorting
   */
  const getFilterQueryString = (filterName: string, filterValue: string[]) => {
    return (
      '?' +
      addNewFilterToParams(searchParams, filterName, filterValue).toString()
    );
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
          maxWidth: 1100,
        }}
      >
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 0.2,
          }}
        >
          <Typography variant="subhead1">Total</Typography>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-around',
              marginTop: 5,
            }}
          >
            <SingleMetric
              name="Devices"
              value={countQuery.data?.total}
              loading={countQuery.isPending}
            />
          </div>
        </div>
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 0.3,
          }}
        >
          <div css={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Typography variant="subhead1">Lease state</Typography>
            <LeaseStateInfo />
          </div>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-around',
              marginTop: 5,
              flexWrap: 'wrap',
            }}
          >
            <SingleMetric
              name="Leased"
              value={countQuery.data?.taskState?.busy}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString('state', ['DEVICE_STATE_LEASED'])}
            />
            <SingleMetric
              name="Available"
              value={countQuery.data?.taskState?.idle}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString('state', [
                'DEVICE_STATE_AVAILABLE',
              ])}
            />
          </div>
        </div>
        <div css={{ flexGrow: 1 }}>
          <Typography variant="subhead1">Device state</Typography>
          <div
            css={{
              display: 'flex',
              marginTop: 5,
              marginLeft: 8,
              flexWrap: 'wrap',
            }}
          >
            <SingleMetric
              name="Ready"
              value={countQuery.data?.deviceState?.ready}
              total={countQuery.data?.total}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString('labels.dut_state', ['ready'])}
            />
            <SingleMetric
              name="Need repair"
              value={countQuery.data?.deviceState?.needRepair}
              total={countQuery.data?.total}
              Icon={
                <WarningIcon
                  sx={{ color: colors.yellow[900], marginTop: '-2px' }}
                />
              }
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString('labels.dut_state', [
                'needs_repair',
              ])}
            />
            <SingleMetric
              name="Repair failed"
              value={countQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString('labels.dut_state', [
                'repair_failed',
              ])}
            />
            <SingleMetric
              name="Need manual repair"
              value={countQuery.data?.deviceState?.needManualRepair}
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isPending}
              filterUrl={getFilterQueryString('labels.dut_state', [
                'needs_manual_repair',
              ])}
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
