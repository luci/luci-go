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

import { Box, Button, Skeleton, Typography } from '@mui/material';
import { ReactElement } from 'react';
import { useNavigate } from 'react-router';

import { useFleetAnalytics } from '@/fleet/hooks/use_fleet_analytics';
import { metricTileSx } from '@/fleet/theme/recipes';
import { fontSizes } from '@/fleet/theme/tokens';

type SingleMetricProps = {
  name: string;
  value?: number;
  total?: number;
  Icon?: ReactElement;
  filterUrl?: string;
  handleClick?: () => void;
  loading?: boolean;
  infoTooltip?: React.ReactNode;
};

export function SingleMetric({
  name,
  value,
  total,
  Icon,
  filterUrl,
  handleClick,
  loading,
  infoTooltip,
}: SingleMetricProps) {
  const { trackEvent } = useFleetAnalytics();
  const navigate = useNavigate();
  const percentage = total && typeof value !== 'undefined' ? value / total : -1;

  const renderPercent = () => {
    if (percentage < 0) return <></>;
    return loading ? (
      <Skeleton data-testid="metric-skeleton" width={16} height={18} />
    ) : (
      <Typography variant="body2" sx={{ color: 'text.secondary' }}>
        {percentage.toLocaleString(undefined, {
          style: 'percent',
          maximumFractionDigits: 1,
          minimumFractionDigits: 0,
        })}
      </Typography>
    );
  };

  const content = (
    <Box
      sx={{
        p: 1,
        display: 'flex',
        flexDirection: 'column',
        textAlign: 'left',
        justifyContent: 'flex-start',
        alignItems: 'flex-start',
        minHeight: 90, // Prevent layout shift when loading data.
        color: 'text.primary',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
        <Typography variant="body2">{name}</Typography>
        {infoTooltip && (
          <Box
            role="presentation"
            onClick={(e) => e.stopPropagation()}
            onMouseDown={(e) => e.stopPropagation()}
            onAuxClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.stopPropagation();
              }
            }}
            sx={{ display: 'inline-flex', alignItems: 'center' }}
          >
            {infoTooltip}
          </Box>
        )}
      </Box>
      <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
        {Icon && Icon}
        {loading ? (
          <Skeleton
            data-testid="metric-skeleton"
            variant="text"
            width={34}
            height={36}
          />
        ) : (
          <>
            <Typography
              variant="h3"
              sx={{
                fontSize: {
                  xs: fontSizes.headline,
                  sm: fontSizes.display,
                },
                lineHeight: 1.2,
              }}
            >
              {(value || 0).toLocaleString('en-US')}
            </Typography>
            {total ? (
              <Typography
                variant="caption"
                sx={{
                  textWrap: 'nowrap',
                  color: 'text.secondary',
                }}
              >{` / ${total.toLocaleString('en-US')}`}</Typography>
            ) : (
              <></>
            )}
          </>
        )}
      </Box>
      {renderPercent()}
    </Box>
  );

  const isClickable = handleClick || filterUrl;

  return (
    <div>
      {isClickable ? (
        <Button
          component="div"
          variant="text"
          sx={{
            ...metricTileSx,
            p: 0,
          }}
          onAuxClick={() => {
            trackEvent('main_metric_clicked', {
              componentName: name,
            });
            if (handleClick) {
              handleClick();
            }
            if (filterUrl) {
              window.open(filterUrl, '_blank');
            }
          }}
          onClick={() => {
            trackEvent('main_metric_clicked', {
              componentName: name,
            });
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
