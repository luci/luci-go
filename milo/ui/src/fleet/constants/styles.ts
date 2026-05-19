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

import { Theme } from '@mui/material/styles';

export const METRICS_COLUMN_STYLE = {
  padding: '2px 4px',
};

export const getMetricsGridStyles = (theme: Theme) => {
  const BORDER_STYLE = `1px solid ${theme.palette.divider}`;
  return {
    col1: {
      ...METRICS_COLUMN_STYLE,
      borderRight: { sm: BORDER_STYLE, md: BORDER_STYLE },
      borderBottom: { xs: BORDER_STYLE, sm: BORDER_STYLE, md: 'none' },
    },
    col2: {
      ...METRICS_COLUMN_STYLE,
      borderRight: { md: BORDER_STYLE },
      borderBottom: { xs: BORDER_STYLE, sm: BORDER_STYLE, md: 'none' },
    },
    col3: {
      ...METRICS_COLUMN_STYLE,
      borderRight: { sm: BORDER_STYLE, md: BORDER_STYLE },
      borderBottom: { xs: BORDER_STYLE, sm: 'none', md: 'none' },
    },
    col4: {
      ...METRICS_COLUMN_STYLE,
      borderBottom: { xs: BORDER_STYLE, sm: 'none', md: 'none' },
    },
  };
};
