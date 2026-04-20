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

import AutoFixHighIcon from '@mui/icons-material/AutoFixHigh';
import BuildIcon from '@mui/icons-material/Build';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import RefreshIcon from '@mui/icons-material/Refresh';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  CircularProgress,
  Divider,
  Grid,
  Typography,
} from '@mui/material';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router';

import { useAdminTaskPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';

export const ChromeOSSmartRepair = () => {
  const { id = '' } = useParams();
  const hasAdminTaskPermission = useAdminTaskPermission();
  const fleetConsoleClient = useFleetConsoleClient();
  const queryClient = useQueryClient();

  const { data, isLoading, isError, error, refetch, isFetching } = useQuery({
    queryKey: ['smart-repair', id],
    queryFn: async () => {
      const response = await fleetConsoleClient.GetSmartRepair({
        deviceIds: [id],
      });
      if (response.results && response.results.length > 0) {
        return response.results[0];
      }
      return null;
    },
    enabled: hasAdminTaskPermission === true && id !== '',
    refetchOnWindowFocus: false,
  });

  if (hasAdminTaskPermission === false) {
    return (
      <Alert severity="error">
        <strong>Admin Access Required:</strong> You do not have the required
        permissions to view this content. To access Smart Repair, please request
        membership in the following group:{' '}
        <a
          href="https://ganpati2.corp.google.com/group/fleet-console-admin-tasks-policy.prod"
          target="_blank"
          rel="noreferrer"
        >
          fleet-console-admin-tasks-policy
        </a>
        .
      </Alert>
    );
  }

  if (hasAdminTaskPermission === null) {
    return <Typography>Checking permissions...</Typography>;
  }

  const loading = isLoading || isFetching;

  return (
    <Box sx={{ mt: 3 }}>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          mb: 3,
        }}
      >
        <Typography
          variant="h6"
          sx={{ display: 'flex', alignItems: 'center', gap: 1 }}
        >
          <AutoFixHighIcon color="primary" />
          AI Analysis Results
        </Typography>
        <Button
          variant="outlined"
          startIcon={<RefreshIcon />}
          onClick={() => {
            queryClient.invalidateQueries({ queryKey: ['smart-repair', id] });
            refetch();
          }}
          disabled={loading}
        >
          Refresh Analysis
        </Button>
      </Box>

      {isError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to fetch Smart Repair data:{' '}
          {error instanceof Error ? error.message : String(error)}
        </Alert>
      )}

      {loading && (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
          <CircularProgress />
        </Box>
      )}

      {!loading && !isError && data && (
        <>
          {data.alreadyInProgress && !data.cachedResult && (
            <Alert severity="info" sx={{ mb: 3 }}>
              An analysis is currently in progress. Please wait and refresh
              later.
            </Alert>
          )}

          {!data.alreadyInProgress && !data.cachedResult && (
            <Alert severity="info" sx={{ mb: 3 }}>
              No cached analysis found for this device. Click &quot;Refresh
              Analysis&quot; to trigger a new request.
            </Alert>
          )}

          {data.cachedResult && (
            <Grid container spacing={3}>
              <Grid item xs={12} md={5}>
                <Card
                  variant="outlined"
                  sx={{ mb: 3, backgroundColor: '#f9f9f9' }}
                >
                  <CardContent>
                    <Typography
                      variant="subtitle2"
                      color="text.secondary"
                      gutterBottom
                    >
                      Conclusion
                    </Typography>
                    {data.cachedResult.conclusions &&
                    data.cachedResult.conclusions.length > 0 ? (
                      data.cachedResult.conclusions.map((conclusion, index) => (
                        <Accordion
                          key={index}
                          disableGutters
                          elevation={0}
                          sx={{
                            '&:before': { display: 'none' },
                            border: '1px solid #e0e0e0',
                            mb: -1,
                            '&:first-of-type': { mt: 1 },
                          }}
                        >
                          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                            <Box
                              sx={{
                                display: 'flex',
                                justifyContent: 'space-between',
                                alignItems: 'center',
                                width: '100%',
                              }}
                            >
                              <Typography
                                sx={{
                                  display: 'flex',
                                  alignItems: 'center',
                                  gap: 1,
                                }}
                              >
                                {conclusion.status
                                  .toLowerCase()
                                  .includes('fail') ||
                                conclusion.status
                                  .toLowerCase()
                                  .includes('broken') ? (
                                  <Alert
                                    icon={false}
                                    severity="error"
                                    sx={{
                                      p: 0,
                                      '& .MuiAlert-message': {
                                        p: 0,
                                        minWidth: 20,
                                        textAlign: 'center',
                                      },
                                    }}
                                  >
                                    !
                                  </Alert>
                                ) : (
                                  <Alert
                                    icon={false}
                                    severity="success"
                                    sx={{
                                      p: 0,
                                      '& .MuiAlert-message': {
                                        p: 0,
                                        minWidth: 20,
                                        textAlign: 'center',
                                      },
                                    }}
                                  >
                                    ✓
                                  </Alert>
                                )}
                                {conclusion.target}
                              </Typography>
                              <Chip
                                label={conclusion.status}
                                size="small"
                                color={
                                  conclusion.status
                                    .toLowerCase()
                                    .includes('fail') ||
                                  conclusion.status
                                    .toLowerCase()
                                    .includes('broken')
                                    ? 'error'
                                    : 'success'
                                }
                                variant="outlined"
                              />
                            </Box>
                          </AccordionSummary>
                          <AccordionDetails>
                            <Typography variant="body2">
                              {conclusion.summary}
                            </Typography>
                            {conclusion.recoveries &&
                              conclusion.recoveries.length > 0 && (
                                <Box sx={{ mt: 1 }}>
                                  <Typography
                                    variant="caption"
                                    color="text.secondary"
                                  >
                                    Recoveries attempted:
                                  </Typography>
                                  <ul>
                                    {conclusion.recoveries.map((rec, i) => (
                                      <li key={i}>
                                        <Typography variant="body2">
                                          {rec}
                                        </Typography>
                                      </li>
                                    ))}
                                  </ul>
                                </Box>
                              )}
                          </AccordionDetails>
                        </Accordion>
                      ))
                    ) : (
                      <Typography variant="body2">
                        No conclusions available.
                      </Typography>
                    )}
                  </CardContent>
                </Card>
              </Grid>

              <Grid item xs={12} md={7}>
                <Card
                  variant="outlined"
                  sx={{
                    borderColor: 'primary.main',
                    backgroundColor: '#f0f7ff',
                  }}
                >
                  <CardContent>
                    <Typography
                      variant="h6"
                      color="primary"
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                        mb: 2,
                      }}
                    >
                      <BuildIcon fontSize="small" />
                      Suggested Repair Steps
                    </Typography>

                    {data.cachedResult.summary && (
                      <Typography
                        variant="body2"
                        sx={{ mb: 2, fontStyle: 'italic' }}
                      >
                        {data.cachedResult.summary}
                      </Typography>
                    )}

                    {data.cachedResult.manualRepairActions &&
                    data.cachedResult.manualRepairActions.length > 0 ? (
                      <Box component="ol" sx={{ pl: 2, m: 0 }}>
                        {data.cachedResult.manualRepairActions.map(
                          (action, index) => (
                            <Typography
                              component="li"
                              variant="body1"
                              key={index}
                              sx={{ mb: 1 }}
                            >
                              {action.replace(/^\d+\.\s*/, '')}
                            </Typography>
                          ),
                        )}
                      </Box>
                    ) : (
                      <Typography variant="body1">
                        No manual repair actions suggested.
                      </Typography>
                    )}

                    {data.cachedResult.logsPath && (
                      <>
                        <Divider sx={{ my: 2 }} />
                        <Typography variant="body2">
                          <strong>Logs Path:</strong>{' '}
                          <a
                            href={data.cachedResult.logsPath}
                            target="_blank"
                            rel="noreferrer"
                          >
                            {data.cachedResult.logsPath}
                          </a>
                        </Typography>
                      </>
                    )}
                  </CardContent>
                </Card>
              </Grid>
            </Grid>
          )}
        </>
      )}
    </Box>
  );
};
