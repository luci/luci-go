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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Typography,
} from '@mui/material';

import { StringPair } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

interface TagsSectionProps {
  tags?: readonly StringPair[];
  panelId: string;
  headerId: string;
}

export function TagsSection({
  tags,
  panelId,
  headerId,
}: TagsSectionProps): JSX.Element {
  return (
    <Accordion>
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        aria-controls={panelId}
        id={headerId}
      >
        <Typography
          sx={{
            fontWeight: 500,
            fontSize: '1rem',
            color: 'var(--gm3-color-on-surface-strong)',
          }}
        >
          Tags ({tags?.length || 0})
        </Typography>
      </AccordionSummary>
      <AccordionDetails id={panelId} sx={{ pl: 2, pr: 2, pt: 1, pb: 1 }}>
        {tags && tags.length > 0 ? (
          <Box component="ul" sx={{ listStyle: 'none', padding: 0, margin: 0 }}>
            {tags.map((tag, index) => (
              <Box
                component="li"
                key={`${tag.key}-${index}`}
                sx={{
                  display: 'flex',
                  mb: 0.5,
                  alignItems: 'flex-start',
                }}
              >
                <Typography
                  variant="body2"
                  sx={{
                    fontWeight: 'medium',
                    color: 'text.secondary',
                    mr: 1,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {tag.key}:
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    color: 'text.primary',
                    wordBreak: 'break-word',
                    whiteSpace: 'pre-wrap',
                  }}
                >
                  {tag.value}
                </Typography>
              </Box>
            ))}
          </Box>
        ) : (
          <Typography variant="body2" color="text.secondary">
            No tags available for this test attempt.
          </Typography>
        )}
      </AccordionDetails>
    </Accordion>
  );
}
