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

import styled from '@emotion/styled';
import ErrorIcon from '@mui/icons-material/Error';
import WarningIcon from '@mui/icons-material/Warning';
import { Alert, Button, Skeleton, Typography } from '@mui/material';
import { UseQueryResult } from '@tanstack/react-query';
import { ReactElement } from 'react';
import { useNavigate } from 'react-router-dom';

import { colors } from '@/fleet/theme/colors';
import { theme } from '@/fleet/theme/theme';
import { SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';
import { addOrUpdateQueryParam } from '@/fleet/utils/search_param';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { CountDevicesResponse } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import {
  FILTERS_PARAM_KEY,
  stringifyFilters,
} from '../multi_select_filter/search_param_utils/search_param_utils';

const Container = styled.div`
  padding: 16px 21px;
  gap: 28;
  border: 1px solid ${colors.grey[300]};
  border-radius: 4;
`;

const HAS_RIGHT_SIBLING_STYLES = {
  borderRight: `1px solid ${colors.grey[300]}`,
  marginRight: 32,
};

interface MainMetricsProps {
  countQuery: UseQueryResult<CountDevicesResponse, unknown>;
  selectedFilters: SelectedOptions | undefined;
}

// TODO: b/393624377 - Refactor this component to make it easier to test.
export function MainMetrics({ countQuery, selectedFilters }: MainMetricsProps) {
  selectedFilters = selectedFilters ?? {};
  const [searchParams, _] = useSyncedSearchParams();

  /**
   * @param filterName name of the filter, ex. state
   * @param filterValue array of filter values, ex. DEVICE_STATE_LEASED
   * @returns will return URL query part with filters and existing parameters, like sorting
   */
  const getFilterQueryString = (filterName: string, filterValue: string[]) => {
    return (
      '?' +
      addOrUpdateQueryParam(
        searchParams,
        FILTERS_PARAM_KEY,
        stringifyFilters({
          ...selectedFilters,
          [filterName]: filterValue,
        }).toString(),
      )
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
              loading={countQuery.isLoading}
            />
          </div>
        </div>
        <div
          css={{
            ...HAS_RIGHT_SIBLING_STYLES,
            flexGrow: 0.4,
          }}
        >
          {/* Changed Task status to Lease state as it is what we show right now,
          still confirming whether this matches with the Task state. Will be changed
          accordingly we clear out everything about this */}
          <Typography variant="subhead1">Lease state</Typography>
          <div
            css={{
              display: 'flex',
              justifyContent: 'space-around',
              marginTop: 5,
            }}
          >
            <SingleMetric
              name="Leased"
              value={countQuery.data?.taskState?.busy}
              total={countQuery.data?.total}
              loading={countQuery.isLoading}
              filterUrl={getFilterQueryString('state', ['DEVICE_STATE_LEASED'])}
            />
            <SingleMetric
              name="Available"
              value={countQuery.data?.taskState?.idle}
              total={countQuery.data?.total}
              loading={countQuery.isLoading}
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
            }}
          >
            <SingleMetric
              name="Ready"
              value={countQuery.data?.deviceState?.ready}
              total={countQuery.data?.total}
              loading={countQuery.isLoading}
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
              loading={countQuery.isLoading}
              filterUrl={getFilterQueryString('labels.dut_state', [
                'needs_repair',
              ])}
            />
            <SingleMetric
              name="Repair failed"
              value={countQuery.data?.deviceState?.repairFailed}
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isLoading}
              filterUrl={getFilterQueryString('labels.dut_state', [
                'repair_failed',
              ])}
            />
            <SingleMetric
              name="Need manual repair"
              value={countQuery.data?.deviceState?.needManualRepair}
              total={countQuery.data?.total}
              Icon={<ErrorIcon sx={{ color: colors.red[600] }} />}
              loading={countQuery.isLoading}
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
    <Container>
      <Typography variant="h4">Main metrics</Typography>
      <div css={{ marginTop: 24 }}>{getContent()}</div>
    </Container>
  );
}

type SingleMetricProps = {
  name: string;
  value?: number;
  total?: number;
  Icon?: ReactElement;
  filterUrl?: string;
  loading?: boolean;
};

export function SingleMetric({
  name,
  value,
  total,
  Icon,
  filterUrl,
  loading,
}: SingleMetricProps) {
  const navigate = useNavigate();
  const percentage = total && typeof value !== 'undefined' ? value / total : -1;

  const renderPercent = () => {
    if (percentage < 0) return <></>;
    return loading ? (
      <Skeleton width={16} height={18} />
    ) : (
      <Typography variant="caption" color={colors.grey[700]}>
        {percentage.toLocaleString(undefined, {
          style: 'percent',
          maximumFractionDigits: 1,
          minimumFractionDigits: 0,
        })}
      </Typography>
    );
  };

  const content = (
    <>
      <Typography variant="body2">{name}</Typography>
      <div css={{ display: 'flex', gap: 4, alignItems: 'center' }}>
        {Icon && Icon}
        {loading ? (
          <Skeleton variant="text" width={34} height={36} />
        ) : (
          <>
            <Typography variant="h3">{value || 0}</Typography>
            {total ? (
              <Typography variant="caption">{` / ${total}`}</Typography>
            ) : (
              <></>
            )}
          </>
        )}
      </div>
      {renderPercent()}
    </>
  );

  return (
    <>
      {filterUrl && (
        <Button
          variant="text"
          sx={{
            marginRight: 'auto',
            display: 'block',
            textAlign: 'left',
            color: theme.palette.text.primary,
          }}
          onClick={() => {
            navigate(filterUrl);
          }}
        >
          {content}
        </Button>
      )}
      {!filterUrl && <div css={{ marginRight: 'auto' }}>{content}</div>}
    </>
  );
}
