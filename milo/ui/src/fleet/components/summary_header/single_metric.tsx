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
              <Typography
                variant="caption"
                sx={{ textWrap: 'nowrap' }}
              >{` / ${total}`}</Typography>
            ) : (
              <></>
            )}
          </>
        )}
      </div>
      {renderPercent()}
    </>
  );

  const isClickable = handleClick || filterUrl;

  return (
    <>
      {isClickable && (
        <Button
          variant="text"
          sx={{
            marginRight: 'auto',
            display: 'block',
            textAlign: 'left',
            color: theme.palette.text.primary,
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
      )}
      {!isClickable && <div css={{ marginRight: 'auto' }}>{content}</div>}
    </>
  );
}
