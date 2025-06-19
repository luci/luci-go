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
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  IconButton,
  Tooltip,
  Typography,
} from '@mui/material';

interface CollapsibleSectionProps {
  title: string;
  children: React.ReactNode;
  defaultExpanded?: boolean;
  helpText?: string;
}

/**
 * A collapsible section in the artifact summary view.
 *
 * This is mostly just an accordion, but has some styling specific to its location in the test investigate page.
 */
export function CollapsibleArtifactSummarySection({
  title,
  children,
  defaultExpanded = true,
  helpText,
}: CollapsibleSectionProps) {
  const panelId = `${title.toLowerCase().replace(/\s+/g, '-')}-panel`;
  const headerId = `${title.toLowerCase().replace(/\s+/g, '-')}-header`;

  return (
    <Accordion
      defaultExpanded={defaultExpanded}
      sx={{
        '&:before': { display: 'none' },
        boxShadow: 'none',
        border: 'none',
        borderBottom: '1px solid var(--divider-color)',
      }}
    >
      <AccordionSummary
        expandIcon={<ExpandMoreIcon />}
        aria-controls={panelId}
        id={headerId}
        sx={{ paddingLeft: 1, paddingRight: 0 }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <Typography variant="body1" sx={{ fontWeight: 'bold' }}>
            {title}
          </Typography>
          {helpText && (
            <Tooltip title={helpText}>
              <IconButton
                size="small"
                sx={{ ml: 0.5 }}
                aria-label={`Info for ${title}`}
                onClick={(e) => e.stopPropagation()}
              >
                <HelpOutlineIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </AccordionSummary>
      <AccordionDetails
        sx={{
          paddingLeft: 0,
          paddingRight: 0,
          paddingTop: 0,
          paddingBottom: 1,
        }}
        id={panelId}
      >
        {children}
      </AccordionDetails>
    </Accordion>
  );
}
