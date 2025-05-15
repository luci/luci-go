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

import BugReportIcon from '@mui/icons-material/BugReport';
import { Box, Link, Typography } from '@mui/material';
import { JSX } from 'react';

import { StyledActionBlock } from '@/common/components/gm3_styled_components';
import { AssociatedBug } from '@/proto/go.chromium.org/luci/analysis/proto/v1/common.pb';

interface NextStepsSubsectionProps {
  associatedBugs?: readonly AssociatedBug[];
}

export function NextStepsSubsection({
  associatedBugs,
}: NextStepsSubsectionProps): JSX.Element {
  return (
    <Box sx={{ flex: 1 }}>
      <Typography
        variant="subtitle1"
        component="div"
        gutterBottom
        sx={{ fontWeight: 'medium' }}
      >
        Next steps
      </Typography>

      {associatedBugs && associatedBugs.length > 0 ? (
        associatedBugs.map((bug) => (
          <StyledActionBlock
            severity="info"
            key={`track-${bug.system}-${bug.id}`}
            sx={{ mb: 1.5 }}
          >
            <BugReportIcon sx={{ mr: 1 }} />
            <Typography variant="body2">
              <Link
                href={bug.url || `https://${bug.system}.com/${bug.id}`}
                target="_blank"
                rel="noopener noreferrer"
              >
                Track {bug.linkText || `${bug.system}/${bug.id}`}
              </Link>
            </Typography>
          </StyledActionBlock>
        ))
      ) : (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
          No associated bugs found for next steps.
        </Typography>
      )}
    </Box>
  );
}
