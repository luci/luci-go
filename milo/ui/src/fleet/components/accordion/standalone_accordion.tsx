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

import { Accordion, AccordionProps } from '@mui/material';

export const StandaloneAccordion = ({
  disableGutters = true,
  elevation = 2,
  variant = 'outlined',
  sx,
  ...props
}: AccordionProps) => (
  <Accordion
    disableGutters={disableGutters}
    elevation={elevation}
    variant={variant}
    sx={{
      mb: 2,
      borderRadius: '4px',
      '&::before': {
        display: 'none',
      },
      ...sx,
    }}
    {...props}
  />
);
