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
import ThumbDownIcon from '@mui/icons-material/ThumbDown';
import ThumbUpIcon from '@mui/icons-material/ThumbUp';
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
  Fade,
  Grid,
  Typography,
} from '@mui/material';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  DocumentSnapshot,
  FirestoreError,
  doc,
  onSnapshot,
} from 'firebase/firestore';
import { useEffect, useState, useCallback } from 'react';
import { useParams } from 'react-router';

import { genFeedbackUrl } from '@/common/tools/utils';
import { db } from '@/firebase';
import { useAdminTaskPermission } from '@/fleet/components/actions/shared/use_admin_task_permission';
import { FEEDBACK_BUGANIZER_BUG_ID } from '@/fleet/constants/feedback';
import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import { GetSmartRepairResult } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import {
  convertGsToHttp,
  formatLogTimestamp,
  getConclusionIcon,
  getConclusionSeverity,
  getHeaderStatusLabel,
  getNormalizedResult,
  SmartRepairRealtimeData,
} from './chromeos_smart_repair_utils';

const CACHE_EXPIRATION_MS = 7 * 24 * 60 * 60 * 1000; // 7 days

export const ChromeOSSmartRepair = () => {
  const { id = '' } = useParams();
  const hasAdminTaskPermission = useAdminTaskPermission();
  const fleetConsoleClient = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const { trackEvent } = useGoogleAnalytics();

  const [feedback, setFeedback] = useState<'up' | 'down' | null>(null);
  const [realtimeData, setRealtimeData] =
    useState<SmartRepairRealtimeData | null>(null);
  const [realtimeError, setRealtimeError] = useState<string | null>(null);
  const [sessionError, setSessionError] = useState<{ message: string } | null>(
    null,
  );

  useEffect(() => {
    setSessionError(null);
  }, [id]);

  useEffect(() => {
    if (hasAdminTaskPermission === true) {
      trackEvent('smart_repair_tab_viewed', {
        componentName: 'smart_repair_tab',
      });
    }
  }, [id, hasAdminTaskPermission, trackEvent]);

  const {
    data,
    isLoading: isInitialLoading,
    isError: isInitialError,
    error: initialError,
    refetch,
    isFetching: isInitialFetching,
  } = useQuery({
    queryKey: ['smart-repair', id],
    queryFn: async () => {
      const response = await fleetConsoleClient.GetSmartRepair({
        deviceIds: [id],
        forceRetrigger: false,
        checkOnly: true,
      });
      if (response.results && response.results.length > 0) {
        return response.results[0] as GetSmartRepairResult;
      }
      return null;
    },
    enabled: hasAdminTaskPermission === true && id !== '',
    refetchInterval: (query) => {
      const resData = query.state.data;
      if (resData && resData.alreadyInProgress && !resData.cachedResult) {
        return 3000;
      }
      return false;
    },
    refetchOnWindowFocus: false,
  });

  const retriggerMutation = useMutation({
    mutationFn: () =>
      fleetConsoleClient.GetSmartRepair({
        deviceIds: [id],
        forceRetrigger: true,
        checkOnly: false,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['smart-repair', id] });
      setRealtimeData(null);
      setRealtimeError(null);
      setSessionError(null);
      refetch();
    },
    onError: (error: Error) => {
      // eslint-disable-next-line no-console
      console.error('Failed to force retrigger:', error);
    },
  });

  const eventId = data?.eventId;
  const hasCachedResult = !!data?.cachedResult;
  const alreadyInProgress = data?.alreadyInProgress;

  useEffect(() => {
    setFeedback(null);
  }, [eventId]);

  const handleFeedback = useCallback(
    (savedTime: boolean) => {
      if (!eventId) return;
      trackEvent('smart_repair_feedback', {
        eventId,
        feedback: savedTime ? 'up' : 'down',
      });
      setFeedback(savedTime ? 'up' : 'down');
    },
    [eventId, trackEvent],
  );

  useEffect(() => {
    if (!eventId) {
      setRealtimeData(null);
      setRealtimeError(null);
      return;
    }

    // If we already have a cached result, we don't need a Firestore listener.
    // We can also clear the realtimeData since displayData will fall back to data.cachedResult.
    if (hasCachedResult) {
      setRealtimeData(null);
      setRealtimeError(null);
      return;
    }

    if (alreadyInProgress) {
      setRealtimeData({ status: 'processing' });
    } else {
      setRealtimeData(null);
    }
    setRealtimeError(null);

    const docRef = doc(db, 'aiRepairResults', eventId);

    const unsubscribe = onSnapshot(
      docRef,
      (docSnap: DocumentSnapshot) => {
        if (docSnap.exists()) {
          const docData = docSnap.data() as SmartRepairRealtimeData;
          setRealtimeData(docData);
          if (docData.status === 'completed' || docData.status === 'error') {
            queryClient.invalidateQueries({ queryKey: ['smart-repair', id] });
          }
          if (docData.status === 'error') {
            setSessionError({
              message: docData.error?.message || 'Unknown error',
            });
          } else if (docData.status === 'completed') {
            setSessionError(null);
          }
          setRealtimeError(null);
        } else {
          if (!alreadyInProgress) {
            setRealtimeError(`Analysis record (ID: ${eventId}) not found.`);
          }
        }
      },
      (err: FirestoreError) => {
        // eslint-disable-next-line no-console
        console.error('Firestore listener error:', err);
        setRealtimeError('Error fetching real-time updates.');
      },
    );

    return () => unsubscribe();
  }, [eventId, id, queryClient, hasCachedResult, alreadyInProgress]);

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

  const isLoading = isInitialLoading || (isInitialFetching && !data);
  const displayData =
    realtimeData ||
    (data?.cachedResult
      ? { status: 'completed', result: data.cachedResult }
      : null);
  const currentStatus =
    realtimeData?.status ||
    (data?.cachedResult
      ? 'completed'
      : data?.alreadyInProgress
        ? 'processing'
        : 'idle');
  const normalizedResult = getNormalizedResult(displayData?.result);
  const logTimestamp = formatLogTimestamp(normalizedResult.logsPath);

  const getTriggeredAndExpiredTimes = () => {
    const ts = realtimeData?.requestTimestamp;
    if (!ts) return null;

    let date: Date;
    if (typeof ts.toDate === 'function') {
      date = ts.toDate();
    } else if (ts.seconds) {
      date = new Date(ts.seconds * 1000);
    } else {
      date = new Date(ts as unknown as string | number | Date);
    }

    if (isNaN(date.getTime())) return null;

    const triggeredStr = date.toLocaleString();

    const expireDate = new Date(date.getTime() + CACHE_EXPIRATION_MS);
    const expiredStr = expireDate.toLocaleString();

    return { triggeredStr, expiredStr };
  };

  const timeInfo = getTriggeredAndExpiredTimes();

  return (
    <Box sx={{ mt: 3 }}>
      <Box
        sx={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          mb: 3,
          flexWrap: 'wrap',
          gap: 2,
        }}
      >
        <Typography
          variant="h6"
          sx={{ display: 'flex', alignItems: 'center', gap: 1 }}
        >
          <AutoFixHighIcon color="primary" />
          AI Analysis Results
          {(currentStatus === 'pending' || currentStatus === 'processing') && (
            <Chip
              label="IN PROGRESS"
              size="small"
              color="warning"
              variant="outlined"
              sx={{ ml: 1 }}
            />
          )}
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          {currentStatus === 'completed' && eventId && (
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1.5,
                p: 1,
                px: 2,
                borderRadius: 2,
                backgroundColor: 'rgba(0, 0, 0, 0.02)',
                border: '1px solid rgba(0, 0, 0, 0.08)',
              }}
            >
              <Typography
                variant="body2"
                sx={{ fontWeight: 500, color: 'text.secondary' }}
              >
                Did this save time?
              </Typography>
              {feedback === null ? (
                <Fade in={feedback === null} timeout={300}>
                  <Box sx={{ display: 'flex', gap: 1 }}>
                    <Button
                      size="small"
                      variant="outlined"
                      color="success"
                      aria-label="Yes, it saved time"
                      sx={{
                        minWidth: 0,
                        p: '4px 8px',
                        borderRadius: 1.5,
                        borderColor: 'rgba(46, 125, 50, 0.3)',
                        '&:hover': {
                          backgroundColor: 'rgba(46, 125, 50, 0.08)',
                          borderColor: 'success.main',
                        },
                      }}
                      onClick={() => handleFeedback(true)}
                    >
                      <ThumbUpIcon sx={{ fontSize: 18 }} />
                    </Button>
                    <Button
                      size="small"
                      variant="outlined"
                      color="error"
                      aria-label="No, it did not"
                      sx={{
                        minWidth: 0,
                        p: '4px 8px',
                        borderRadius: 1.5,
                        borderColor: 'rgba(211, 47, 47, 0.3)',
                        '&:hover': {
                          backgroundColor: 'rgba(211, 47, 47, 0.08)',
                          borderColor: 'error.main',
                        },
                      }}
                      onClick={() => handleFeedback(false)}
                    >
                      <ThumbDownIcon sx={{ fontSize: 18 }} />
                    </Button>
                  </Box>
                </Fade>
              ) : (
                <Fade in={feedback !== null} timeout={300}>
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 0.5,
                      fontWeight: 600,
                    }}
                  >
                    Thank you!
                  </Typography>
                </Fade>
              )}
            </Box>
          )}
          <Button
            variant="outlined"
            startIcon={
              retriggerMutation.isPending ? (
                <CircularProgress size={20} />
              ) : (
                <RefreshIcon />
              )
            }
            onClick={() => {
              trackEvent('run_smart_repair', {
                componentName: 'retrigger_analysis_button',
              });
              setSessionError(null);
              retriggerMutation.mutate();
            }}
            disabled={
              isLoading ||
              retriggerMutation.isPending ||
              currentStatus === 'pending' ||
              currentStatus === 'processing'
            }
          >
            {retriggerMutation.isPending
              ? 'Retriggering...'
              : 'Retrigger Analysis'}
          </Button>
        </Box>
      </Box>

      {timeInfo && (
        <Box sx={{ display: 'flex', gap: 3, mb: 3, mt: -1 }}>
          <Typography variant="caption" color="text.secondary">
            <strong>Triggered:</strong> {timeInfo.triggeredStr}
          </Typography>
          <Typography variant="caption" color="text.secondary">
            <strong>Expires:</strong> {timeInfo.expiredStr}
          </Typography>
        </Box>
      )}

      {isInitialError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to fetch Smart Repair data:{' '}
          {initialError instanceof Error
            ? initialError.message
            : String(initialError)}
        </Alert>
      )}
      {realtimeError && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Real-time Update Error: {realtimeError}
        </Alert>
      )}

      {isLoading && (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
          <CircularProgress />
          <Typography sx={{ ml: 2 }}>
            Loading analysis data... This will take a few minutes.
          </Typography>
        </Box>
      )}

      {!isLoading && !isInitialError && (
        <>
          {currentStatus === 'idle' && !displayData && !sessionError && (
            <Alert severity="info" sx={{ mb: 3 }}>
              No active or cached analysis found for this device. Click
              &quot;Trigger/Refresh Analysis&quot; to start.
            </Alert>
          )}

          {sessionError && (
            <Alert severity="error" sx={{ mb: 3 }}>
              Analysis resulted in error. {sessionError.message}. Please
              retrigger a new test. If this behavior is persistent, please{' '}
              <a
                href={genFeedbackUrl({
                  bugComponent: FEEDBACK_BUGANIZER_BUG_ID,
                })}
                target="_blank"
                rel="noreferrer"
              >
                file a bug
              </a>
              .
            </Alert>
          )}

          {(currentStatus === 'pending' || currentStatus === 'processing') &&
            !displayData?.result && (
              <Alert severity="info" sx={{ mb: 3 }}>
                Analysis is currently {currentStatus}.
                <CircularProgress size={16} sx={{ ml: 2 }} />
              </Alert>
            )}

          {displayData?.result && (
            <Box>
              {/* Summary banner right under the title */}
              {normalizedResult.summary && (
                <Box
                  sx={{
                    p: 2.5,
                    mb: 3,
                    borderRadius: 2,
                    backgroundColor: '#eef6fc',
                    borderLeft: '5px solid #1976d2',
                  }}
                >
                  <Typography
                    variant="subtitle2"
                    color="primary"
                    sx={{ fontWeight: 'bold', mb: 0.5 }}
                  >
                    Analysis Summary
                  </Typography>
                  <Typography variant="body1" color="text.primary">
                    {normalizedResult.summary}
                  </Typography>
                </Box>
              )}

              {/* Split Layout Grid */}
              <Grid container spacing={3}>
                {/* Left Side Column (Logs + Conclusions) */}
                <Grid item xs={12} md={6}>
                  {/* Task Log Link Div */}
                  {normalizedResult.logsPath && (
                    <Box
                      sx={{
                        p: 2,
                        mb: 3,
                        borderRadius: 1,
                        backgroundColor: '#f5f5f5',
                        border: '1px solid #e0e0e0',
                      }}
                    >
                      <Typography
                        variant="subtitle2"
                        color="text.secondary"
                        gutterBottom
                      >
                        Latest Task Log
                      </Typography>
                      <Box
                        sx={{
                          display: 'flex',
                          flexDirection: 'column',
                          gap: 0.5,
                        }}
                      >
                        <a
                          href={convertGsToHttp(normalizedResult.logsPath)}
                          target="_blank"
                          rel="noreferrer"
                          style={{
                            wordBreak: 'break-all',
                            textDecoration: 'none',
                            color: '#1976d2',
                            fontWeight: 500,
                          }}
                        >
                          {normalizedResult.logsPath}
                        </a>
                        {logTimestamp && (
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{ mt: 0.5 }}
                          >
                            Log Date: {logTimestamp}
                          </Typography>
                        )}
                      </Box>
                    </Box>
                  )}

                  {/* Conclusions Card */}
                  <Card variant="outlined" sx={{ backgroundColor: '#f9f9f9' }}>
                    <CardContent>
                      <Typography
                        variant="subtitle2"
                        color="text.secondary"
                        gutterBottom
                      >
                        Conclusion
                      </Typography>
                      {normalizedResult.conclusions &&
                      normalizedResult.conclusions.length > 0 ? (
                        normalizedResult.conclusions.map(
                          (conclusion, index) => (
                            <Accordion
                              key={index}
                              disableGutters
                              elevation={0}
                              sx={{
                                '&:before': { display: 'none' },
                                border: '1px solid #e0e0e0',
                                mb:
                                  index ===
                                  normalizedResult.conclusions.length - 1
                                    ? 0
                                    : 1,
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
                                  <Box
                                    sx={{
                                      display: 'flex',
                                      alignItems: 'center',
                                      gap: 1,
                                    }}
                                  >
                                    <Alert
                                      icon={false}
                                      severity={getConclusionSeverity(
                                        conclusion.status,
                                      )}
                                      sx={{
                                        p: 0,
                                        '& .MuiAlert-message': {
                                          p: 0,
                                          minWidth: 20,
                                          textAlign: 'center',
                                        },
                                      }}
                                    >
                                      {getConclusionIcon(
                                        getConclusionSeverity(
                                          conclusion.status,
                                        ),
                                      )}
                                    </Alert>
                                    {conclusion.target}
                                  </Box>
                                  <Chip
                                    label={getHeaderStatusLabel(
                                      conclusion.status,
                                    )}
                                    size="small"
                                    color={getConclusionSeverity(
                                      conclusion.status,
                                    )}
                                    variant="outlined"
                                  />
                                </Box>
                              </AccordionSummary>
                              <AccordionDetails>
                                {conclusion.status && (
                                  <Typography
                                    variant="subtitle2"
                                    color="text.secondary"
                                    component="div"
                                    sx={{ mb: 0.5, fontWeight: 'bold' }}
                                  >
                                    Status: {conclusion.status}
                                  </Typography>
                                )}
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
                          ),
                        )
                      ) : (
                        <Typography variant="body2">
                          No conclusions available.
                        </Typography>
                      )}
                    </CardContent>
                  </Card>
                </Grid>

                {/* Right Side Column (Repair Steps) */}
                <Grid item xs={12} md={6}>
                  <Card
                    variant="outlined"
                    sx={{
                      borderColor: 'primary.main',
                      backgroundColor: '#f0f7ff',
                      height: '100%',
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

                      {normalizedResult.manualRepairActions &&
                      normalizedResult.manualRepairActions.length > 0 ? (
                        <Box component="ol" sx={{ pl: 2, m: 0 }}>
                          {normalizedResult.manualRepairActions.map(
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
                    </CardContent>
                  </Card>
                </Grid>
              </Grid>
            </Box>
          )}
        </>
      )}
    </Box>
  );
};
