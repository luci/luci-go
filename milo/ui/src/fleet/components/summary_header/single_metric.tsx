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

import { Button, colors, Skeleton, Typography } from '@mui/material';
import { ReactElement } from 'react';
import { useNavigate } from 'react-router';

import { theme } from '@/fleet/theme/theme';

type SingleMetricProps = {
  name: string;
  value?: number;
  total?: number;
  Icon?: ReactElement;
  filterUrl?: string;
  handleClick?: () => void;
  loading?: boolean;
};

export function SingleMetric({
  name,
  value,
  total,
  Icon,
  filterUrl,
  handleClick,
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
    <div
      css={{
        padding: 8,
        display: 'flex',
        flexDirection: 'column',
        textAlign: 'left',
        justifyContent: 'flex-start',
        alignItems: 'flex-start',
        minHeight: '90px', // Prevent layout shift when loading data.
        color: theme.palette.text.primary,
      }}
    >
      <Typography variant="body2">{name}</Typography>
      <div css={{ display: 'flex', gap: 4, alignItems: 'center' }}>
        {Icon && Icon}
        {loading ? (
          <Skeleton variant="text" width={34} height={36} />
        ) : (
          <>
            <Typography variant="h3">
              {(value || 0).toLocaleString('en-US')}
            </Typography>
            {total ? (
              <Typography
                variant="caption"
                sx={{ textWrap: 'nowrap' }}
              >{` / ${total.toLocaleString('en-US')}`}</Typography>
            ) : (
              <></>
            )}
          </>
        )}
      </div>
      {renderPercent()}
    </div>
  );

  const isClickable = handleClick || filterUrl;

  return (
    <div>
      {isClickable ? (
        <Button
          variant="text"
          css={{
            padding: 0,
          }}
          onClick={() => {
            if (handleClick) {
              handleClick();
            }
            if (filterUrl) {
              navigate(filterUrl);
            }
          }}
        >
          {content}
        </Button>
      ) : (
        content
      )}
    </div>
  );
}
