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

import BugReportOutlinedIcon from '@mui/icons-material/BugReportOutlined';
import {
  Alert,
  Box,
  Chip,
  CircularProgress,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';

import { EllipsisTooltip } from '@/fleet/components/ellipsis_tooltip';
import { getErrorMessage } from '@/fleet/utils/errors';

import { useIrmIncidents } from './use_irm_incidents';

export const ActiveIrmTable = () => {
  const { data, isLoading, isError, error } = useIrmIncidents();

  const incidents = data?.irmIncidents ?? [];

  return (
    <Box sx={{ width: '100%' }}>
      <Typography
        variant="h6"
        sx={{ mt: 2, mb: 2, fontSize: '16px', fontWeight: 'bold' }}
      >
        Active IRM Bugs
      </Typography>

      {isLoading && (
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            p: 4,
          }}
          data-testid="active-irm-loading"
        >
          <CircularProgress size={28} />
        </Box>
      )}

      {isError && (
        <Alert severity="error" sx={{ my: 1 }} data-testid="active-irm-error">
          {getErrorMessage(error, 'loading active IRM incidents')}
        </Alert>
      )}

      {!isLoading && !isError && (
        <TableContainer
          component={Paper}
          variant="outlined"
          sx={{
            maxHeight: 200,
            overflowY: 'auto',
            borderRadius: 1,
          }}
          data-testid="active-irm-table-container"
        >
          <Table
            size="small"
            stickyHeader
            aria-label="active IRM incidents table"
            sx={{ tableLayout: 'fixed' }}
          >
            <TableHead>
              <TableRow>
                <TableCell sx={{ fontWeight: 'bold', width: '130px' }}>
                  Bug ID
                </TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>Incident Name</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {incidents.length === 0 ? (
                <TableRow>
                  <TableCell
                    colSpan={2}
                    align="center"
                    sx={{ color: 'text.secondary', py: 4 }}
                    data-testid="active-irm-empty"
                  >
                    No active IRM incidents
                  </TableCell>
                </TableRow>
              ) : (
                incidents.map((incident, index) => {
                  const bugId = incident.masterBugId?.trim() ?? '';
                  const title = incident.title?.trim() || '—';
                  return (
                    <TableRow
                      key={incident.id || bugId || `irm-${index}`}
                      hover
                    >
                      <TableCell
                        sx={{ verticalAlign: 'middle', whiteSpace: 'nowrap' }}
                      >
                        {bugId ? (
                          <Chip
                            component="a"
                            href={`https://b.corp.google.com/issues/${encodeURIComponent(bugId)}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            icon={<BugReportOutlinedIcon />}
                            label={'b/' + bugId}
                            clickable
                            size="small"
                            color="error"
                            variant="outlined"
                            sx={{ fontWeight: 600 }}
                            data-testid={`irm-chip-${bugId}`}
                          />
                        ) : (
                          <Typography variant="caption" color="text.secondary">
                            N/A
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell
                        sx={{
                          verticalAlign: 'middle',
                          maxWidth: 0,
                          overflow: 'hidden',
                        }}
                      >
                        <EllipsisTooltip>
                          <Typography variant="body2" noWrap>
                            {title}
                          </Typography>
                        </EllipsisTooltip>
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
};
