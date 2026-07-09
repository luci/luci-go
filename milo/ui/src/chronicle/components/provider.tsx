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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import { ReactNode, useCallback, useEffect, useMemo, useState } from 'react';
import { useParams } from 'react-router';

import {
  TokenType,
  useGetAuthToken,
} from '@/common/components/auth_state_provider';
import { fetchGrpcWeb } from '@/common/hooks/grpc_query/grpc_query';
import {
  TURBO_CI_ENVIRONMENTS,
  TurboCIEnvironment,
  useReadWorkPlan,
} from '@/common/hooks/grpc_query/turbo_ci/turbo_ci';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import { IdentifierKind } from '@/proto/turboci/graph/ids/v1/identifier_kind.pb';
import { ReadWorkPlanRequest } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_request.pb';
import { ReadWorkPlanResponse } from '@/proto/turboci/graph/orchestrator/v1/read_workplan_response.pb';
import { TurboCIOrchestratorServiceName } from '@/proto/turboci/graph/orchestrator/v1/turbo_ci_orchestrator_service.pb';
import { ValueData } from '@/proto/turboci/graph/orchestrator/v1/value_data.pb';
import { ValueMask } from '@/proto/turboci/graph/orchestrator/v1/value_mask.pb';

import { FakeGraphGenerator, WorkflowType } from '../fake_turboci_graph';

import {
  ChronicleContext,
  DEMO_ENVIRONMENT_NAME,
  DEMO_WORKPLAN_ID,
  DetectionErrorType,
  FailedEnvironment,
} from './context';

interface LookupResult {
  exists?: boolean;
  errorType?: DetectionErrorType;
  success?: boolean;
  retryable?: boolean;
}

async function performLookupAttempt(
  host: string,
  workplanId: string,
  accessToken: string,
  signal: AbortSignal,
): Promise<LookupResult> {
  try {
    const result = await fetchGrpcWeb({
      host,
      service: TurboCIOrchestratorServiceName,
      method: 'ReadWorkPlan',
      request: {
        workplanId: { id: workplanId },
        includedNodeTypes: [],
      },
      requestMsg: ReadWorkPlanRequest,
      responseMsg: ReadWorkPlanResponse,
      accessToken,
      signal,
    });

    return { success: true, exists: !!result.workplan };
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      return {
        success: false,
        retryable: false,
        errorType: DetectionErrorType.Timeout,
      };
    }

    if (err instanceof GrpcError) {
      if (err.code === RpcCode.NOT_FOUND) {
        return { success: true, exists: false };
      }
      if (
        err.code === RpcCode.UNAUTHENTICATED ||
        err.code === RpcCode.PERMISSION_DENIED
      ) {
        return {
          success: true,
          exists: false,
          errorType: DetectionErrorType.NoAccess,
        };
      }

      const isRetryable =
        err.code === RpcCode.RESOURCE_EXHAUSTED ||
        err.code === RpcCode.UNAVAILABLE ||
        err.code === RpcCode.INTERNAL;

      return {
        success: false,
        retryable: isRetryable,
        errorType: DetectionErrorType.Error,
      };
    }

    const isNetworkError = err instanceof TypeError;
    return {
      success: false,
      retryable: isNetworkError,
      errorType: DetectionErrorType.Error,
    };
  }
}

async function lookupWorkplanExists(
  host: string,
  workplanId: string,
  accessToken: string,
  parentSignal?: AbortSignal,
  timeoutMs = 5000,
): Promise<LookupResult> {
  const controller = new AbortController();
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

  const abortHandler = () => controller.abort();

  if (parentSignal) {
    parentSignal.addEventListener('abort', abortHandler);
  }

  const maxRetries = 3;
  let delayMs = 200;

  try {
    for (let attempt = 1; attempt <= maxRetries; attempt++) {
      const result = await performLookupAttempt(
        host,
        workplanId,
        accessToken,
        controller.signal,
      );

      if (result.success) {
        return { exists: result.exists!, errorType: result.errorType };
      }

      if (result.errorType === DetectionErrorType.Timeout) {
        return { exists: false, errorType: DetectionErrorType.Timeout };
      }

      if (!result.retryable || attempt === maxRetries) {
        return { exists: false, errorType: DetectionErrorType.Error };
      }

      // eslint-disable-next-line no-console
      console.warn(
        `Retryable error checking workplan on ${host}. ` +
          `Retrying in ${delayMs}ms (attempt ${attempt}/${maxRetries})...`,
      );

      try {
        await new Promise<void>((resolve, reject) => {
          if (controller.signal.aborted) {
            return reject(new DOMException('Aborted', 'AbortError'));
          }

          const onAbort = () => {
            cleanup();
            reject(new DOMException('Aborted', 'AbortError'));
          };

          const onTimeout = () => {
            cleanup();
            resolve();
          };

          const cleanup = () => {
            clearTimeout(timer);
            controller.signal.removeEventListener('abort', onAbort);
          };

          const timer = setTimeout(onTimeout, delayMs);
          controller.signal.addEventListener('abort', onAbort);
        });
      } catch (_) {
        return { exists: false, errorType: DetectionErrorType.Timeout };
      }

      delayMs *= 2;
    }
  } finally {
    clearTimeout(timeoutId);
    if (parentSignal) {
      parentSignal.removeEventListener('abort', abortHandler);
    }
  }

  return { exists: false, errorType: DetectionErrorType.Error };
}

function compareEnvironments(
  a: TurboCIEnvironment,
  b: TurboCIEnvironment,
): number {
  const idxA = TURBO_CI_ENVIRONMENTS.findIndex((e) => e.host === a.host);
  const idxB = TURBO_CI_ENVIRONMENTS.findIndex((e) => e.host === b.host);
  return idxA - idxB;
}

/**
 * Hook to sync node selection with the URL query parameter `nodeId`.
 */
function useNodeSelection() {
  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const selectedNodeId = searchParams.get('nodeId') || undefined;

  const setSelectedNodeId = useCallback(
    (id: string | undefined) => {
      setSearchParams(
        (prev: URLSearchParams) => {
          const next = new URLSearchParams(prev);
          if (id) {
            next.set('nodeId', id);
          } else {
            next.delete('nodeId');
          }
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  return { selectedNodeId, setSelectedNodeId };
}

export function ChronicleContextProvider({
  children,
}: {
  children: ReactNode;
}) {
  const { workplanId } = useParams<{ workplanId: string }>();
  const [workflowType, setWorkflowType] = useState<WorkflowType>(
    WorkflowType.ANDROID,
  );
  const { selectedNodeId, setSelectedNodeId } = useNodeSelection();

  if (!workplanId) {
    throw new Error('Invalid URL: Missing workplanId parameter.');
  }

  const normalizedWorkplanId =
    workplanId.startsWith('L') && workplanId.length > 1
      ? workplanId.substring(1)
      : workplanId;

  const useFakeData = normalizedWorkplanId === DEMO_WORKPLAN_ID;

  const [activeEnvironment, setActiveEnvironmentState] = useState<
    string | undefined
  >(useFakeData ? DEMO_WORKPLAN_ID : undefined);
  const [detecting, setDetecting] = useState(!useFakeData);
  const [detectionFailed, setDetectionFailed] = useState(false);
  const [showEnvDialog, setShowEnvDialog] = useState(false);
  const [detectedEnvironments, setDetectedEnvironments] = useState<
    TurboCIEnvironment[]
  >(
    useFakeData
      ? [
          {
            environment: DEMO_WORKPLAN_ID,
            urlParam: DEMO_WORKPLAN_ID,
            host: DEMO_WORKPLAN_ID,
          },
        ]
      : [],
  );
  const [requestedEnvFailed, setRequestedEnvFailed] = useState<
    string | undefined
  >(undefined);
  const [failedEnvironments, setFailedEnvironments] = useState<
    FailedEnvironment[]
  >([]);
  const [detectionCancelled, setDetectionCancelled] = useState(false);

  const [searchParams, setSearchParams] = useSyncedSearchParams();
  const requestedEnvParam = searchParams.get('env') || undefined;

  const setActiveEnvironment = useCallback(
    (environment: string | undefined) => {
      setActiveEnvironmentState(environment);
      const matchedEnv = TURBO_CI_ENVIRONMENTS.find(
        (e) => e.environment === environment,
      );
      setSearchParams(
        (prev: URLSearchParams) => {
          const next = new URLSearchParams(prev);
          if (matchedEnv && environment !== DEMO_WORKPLAN_ID) {
            next.set('env', matchedEnv.urlParam);
          } else {
            next.delete('env');
          }
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const [completedCount, setCompletedCount] = useState(0);

  const getAccessToken = useGetAuthToken(TokenType.Access);

  // Effect 1: Trigger parallel network scans only when the workplanId changes.
  useEffect(() => {
    if (useFakeData) {
      setDetectedEnvironments([
        {
          environment: DEMO_ENVIRONMENT_NAME,
          urlParam: DEMO_WORKPLAN_ID,
          host: DEMO_WORKPLAN_ID,
        },
      ]);
      setFailedEnvironments([]);
      setCompletedCount(TURBO_CI_ENVIRONMENTS.length);
      return;
    }

    // Reset states for a new scan
    setDetectedEnvironments([]);
    setFailedEnvironments([]);
    setCompletedCount(0);
    setRequestedEnvFailed(undefined);
    setShowEnvDialog(false);
    setDetectionCancelled(false);

    const controller = new AbortController();
    const signal = controller.signal;
    let active = true;

    async function detect() {
      let token = '';
      try {
        token = await getAccessToken();
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('Failed to get access token:', err);
        if (active) {
          setCompletedCount(TURBO_CI_ENVIRONMENTS.length);
        }
        return;
      }

      TURBO_CI_ENVIRONMENTS.forEach(async (env) => {
        const { exists, errorType } = await lookupWorkplanExists(
          env.host,
          normalizedWorkplanId,
          token,
          signal,
        );

        if (!active) return;

        if (exists) {
          setDetectedEnvironments((prev) => {
            if (prev.some((e) => e.host === env.host)) return prev;
            const next = [...prev, env];
            return next.sort(compareEnvironments);
          });
        } else if (errorType) {
          setFailedEnvironments((prev) => {
            if (prev.some((f) => f.env.host === env.host)) return prev;
            const next = [...prev, { env, errorType }];
            return next.sort((a, b) => compareEnvironments(a.env, b.env));
          });
        }

        setCompletedCount((c) => c + 1);
      });
    }

    detect();

    return () => {
      active = false;
      controller.abort();
    };
  }, [normalizedWorkplanId, useFakeData, getAccessToken]);

  // Effect 2: Resolve active environment and status based on scan results and URL state.
  useEffect(() => {
    if (detectionCancelled) return;

    if (useFakeData) {
      setActiveEnvironmentState(DEMO_ENVIRONMENT_NAME);
      setDetecting(false);
      setDetectionFailed(false);
      return;
    }

    const scanComplete = completedCount === TURBO_CI_ENVIRONMENTS.length;
    const requestedEnv = TURBO_CI_ENVIRONMENTS.find(
      (e) => e.urlParam === requestedEnvParam,
    );
    const prodEnv = TURBO_CI_ENVIRONMENTS.find(
      (e) => e.environment === 'prod',
    )!;

    if (requestedEnv) {
      const isRequestedDetected = detectedEnvironments.some(
        (e) => e.host === requestedEnv.host,
      );
      if (isRequestedDetected) {
        setActiveEnvironmentState(requestedEnv.environment);
        setDetecting(false);
        setDetectionFailed(false);
        setShowEnvDialog(false);
        return;
      }

      if (scanComplete) {
        setRequestedEnvFailed(requestedEnv.environment);
        if (detectedEnvironments.length > 0) {
          setShowEnvDialog(true);
          setDetecting(false);
        } else {
          setDetectionFailed(true);
          setDetecting(false);
        }
      } else {
        setDetecting(true);
      }
    } else {
      const isProdDetected = detectedEnvironments.some(
        (e) => e.host === prodEnv.host,
      );
      if (isProdDetected) {
        setActiveEnvironmentState(prodEnv.environment);
        setDetecting(false);
        setDetectionFailed(false);
        setShowEnvDialog(false);
        return;
      }

      if (scanComplete) {
        if (detectedEnvironments.length === 1) {
          setActiveEnvironmentState(detectedEnvironments[0].environment);
          setDetecting(false);
          setDetectionFailed(false);
          setShowEnvDialog(false);
        } else if (detectedEnvironments.length > 1) {
          setShowEnvDialog(true);
          setDetecting(false);
        } else {
          setDetectionFailed(true);
          setDetecting(false);
        }
      } else {
        setDetecting(true);
      }
    }
  }, [
    useFakeData,
    completedCount,
    detectedEnvironments,
    requestedEnvParam,
    detectionCancelled,
  ]);

  const activeEnvObj = TURBO_CI_ENVIRONMENTS.find(
    (e) => e.environment === activeEnvironment,
  );
  const activeHost = activeEnvObj?.host || '';

  const result = useReadWorkPlan(
    {
      workplanId: { id: normalizedWorkplanId },
      includedNodeTypes: [
        IdentifierKind.IDENTIFIER_KIND_CHECK,
        IdentifierKind.IDENTIFIER_KIND_CHECK_EDIT,
        IdentifierKind.IDENTIFIER_KIND_STAGE,
        IdentifierKind.IDENTIFIER_KIND_STAGE_ATTEMPT,
        IdentifierKind.IDENTIFIER_KIND_STAGE_EDIT,
      ],
      valueFilter: {
        typeInfo: {
          wanted: { typeUrls: ['*'] },
          unknownJsonpb: true,
          known: { typeUrls: [] },
        },
        checkOptions: ValueMask.VALUE_MASK_VALUE_TYPE,
        checkResultData: ValueMask.VALUE_MASK_VALUE_TYPE,
        checkEditOptions: ValueMask.VALUE_MASK_VALUE_TYPE,
        checkEditResultData: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageArgs: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageAttemptDetails: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageAttemptProgressDetails: ValueMask.VALUE_MASK_TYPE,
        stageEditAttemptDetails: ValueMask.VALUE_MASK_VALUE_TYPE,
        stageEditAttemptProgressDetails: ValueMask.VALUE_MASK_TYPE,
      },
    },
    activeHost || '',
    {
      enabled: !useFakeData && !!activeHost,
    },
  );

  if (!useFakeData && result.error) {
    throw result.error;
  }

  const workplanValueMap = useMemo(() => {
    if (useFakeData) {
      const generator = new FakeGraphGenerator({
        workPlanIdStr: normalizedWorkplanId,
        workflowType: workflowType,
      });
      return generator.generate();
    }

    const response = result.data;
    const valueDataMap: Map<string, ValueData> = new Map(
      Object.entries(response?.valueData ?? []),
    );

    return {
      workplan: response?.workplan,
      valueDataMap: valueDataMap,
    };
  }, [normalizedWorkplanId, workflowType, useFakeData, result.data]);

  const value = useMemo(
    () => ({
      workplanId: normalizedWorkplanId,
      graph: workplanValueMap.workplan,
      valueDataMap: workplanValueMap.valueDataMap,
      workflowType,
      setWorkflowType,
      selectedNodeId,
      setSelectedNodeId,
      detecting,
      setDetecting,
      detectionFailed,
      setDetectionFailed,
      showEnvDialog,
      setShowEnvDialog,
      detectedEnvironments,
      activeEnvironment,
      setActiveEnvironment,
      requestedEnvFailed,
      failedEnvironments,
      detectionCancelled,
      setDetectionCancelled,
    }),
    [
      normalizedWorkplanId,
      workplanValueMap.workplan,
      workplanValueMap.valueDataMap,
      workflowType,
      setWorkflowType,
      selectedNodeId,
      setSelectedNodeId,
      detecting,
      setDetecting,
      detectionFailed,
      setDetectionFailed,
      showEnvDialog,
      setShowEnvDialog,
      detectedEnvironments,
      activeEnvironment,
      setActiveEnvironment,
      requestedEnvFailed,
      failedEnvironments,
      detectionCancelled,
      setDetectionCancelled,
    ],
  );

  return (
    <ChronicleContext.Provider value={value}>
      {children}
    </ChronicleContext.Provider>
  );
}
