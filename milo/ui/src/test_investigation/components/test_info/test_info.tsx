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

import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown';
import ArrowForwardIcon from '@mui/icons-material/ArrowForward';
import AutorenewIcon from '@mui/icons-material/Autorenew';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import { Box, Button, Card, Chip, Link, Typography } from '@mui/material';

import { StyledActionBlock } from '@/common/components/gm3_styled_components';

export function TestInfo() {
  return (
    <>
      <Typography variant="h5">Top Section Placeholder</Typography>
      <Box
        sx={{
          height: '300px',
          display: 'grid',
          gridTemplateColumns: '1fr 3fr',
          gap: '32px',
        }}
      >
        <Card>
          Overview
          <Chip label="Default Tag" color="default" />
          <Chip label="Running" icon={<AutorenewIcon />} color="primary" />
        </Card>
        <Card>
          <Button variant="outlined" startIcon={<ArrowDropDownIcon />}>
            Add comparison
          </Button>
          <StyledActionBlock severity="primary">
            <ArrowForwardIcon />
            <Typography>
              Investigate the root cause. This text could be a{' '}
              <Link
                href="#"
                sx={{
                  fontWeight: 'bold',
                  color: 'primary.main', // Use theme color
                }}
              >
                link
              </Link>
              .
            </Typography>
          </StyledActionBlock>
          <StyledActionBlock severity="warning">
            <WarningAmberIcon />
            <Typography>
              Another investigation needed with a different severity.
            </Typography>
          </StyledActionBlock>
        </Card>
      </Box>
    </>
  );
}
