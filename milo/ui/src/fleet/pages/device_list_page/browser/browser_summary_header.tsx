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

import { Typography, Alert } from '@mui/material';

import { SingleMetric } from '@/fleet/components/summary_header/single_metric';
import { MetricsContainer } from '@/fleet/constants/css_snippets';
import { SelectedOptions } from '@/fleet/types';
import { getErrorMessage } from '@/fleet/utils/errors';

export interface BrowserSummaryHeaderProps {
  selectedOptions: SelectedOptions;
}

export function BrowserSummaryHeader({}: BrowserSummaryHeaderProps) {
  // TODO: b/390013758 - Use the real CountDevices RPC once it's ready for CHROMIUM platform.
  const countQuery = {
    data: { total: 0 },
    isPending: false,
    isError: false,
    error: null,
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
        <div>
          <Typography variant="subhead1">Device state</Typography>
          <div
            css={{
              display: 'flex',
              justifyContent: 'flex-start',
              marginTop: 4,
              flexWrap: 'wrap',
              gap: 8,
            }}
          >
            <SingleMetric
              name="Total"
              value={countQuery.data?.total}
              loading={countQuery.isPending}
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
