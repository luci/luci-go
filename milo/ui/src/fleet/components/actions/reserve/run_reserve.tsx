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

import LockIcon from '@mui/icons-material/Lock';
import { Button, Snackbar } from '@mui/material';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { useFleetConsoleClient } from '@/fleet/hooks/prpc_clients';
import { useGoogleAnalytics } from '@/generic_libs/components/google_analytics';
import {
  ScheduleReserveRequest,
  ScheduleReserveRequest_ReserveFlag,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { AdminAccessRequiredDialog } from '../shared/admin_access_required_dialog';
import { DutToRepair } from '../shared/types';
import { useAdminTaskPermission } from '../shared/use_admin_task_permission';

import ReserveDialog, { ReserveResult } from './reserve_dialog';

interface RunReserveProps {
  selectedDuts: DutToRepair[];
}

export function RunReserve({ selectedDuts }: RunReserveProps) {
  const { trackEvent } = useGoogleAnalytics();
  const fleetConsoleClient = useFleetConsoleClient();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(false);
  const [results, setResults] = useState<ReserveResult[] | undefined>(
    undefined,
  );
  const [comment, setComment] = useState<string>('');
  const [latest, setLatest] = useState<boolean>(false);
  const [sessionId, setSessionId] = useState<string | undefined>(undefined);
  const [adminAccessRequiredDialogOpen, setAdminAccessRequiredDialogOpen] =
    useState<boolean>(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const { hasPermission, fetchPermissions } = useAdminTaskPermission();
  const [checkingPermission, setCheckingPermission] = useState<boolean>(false);

  const dutNames = selectedDuts.map((d) => d.name);

  const initializeReserve = async () => {
    setCheckingPermission(true);
    try {
      const result = await fetchPermissions();
      if (result.hasPermission === true) {
        setResults(undefined);
        setComment('');
        setLatest(false);
        setSessionId(undefined);
        setLoading(false);
        setOpen(true);
      } else {
        setAdminAccessRequiredDialogOpen(true);
      }
    } catch (e) {
      setErrorMessage(
        e instanceof Error ? e.message : 'Failed to verify permissions.',
      );
    } finally {
      setCheckingPermission(false);
    }
  };

  const handleClose = () => {
    setOpen(false);
    if (results) {
      void queryClient.invalidateQueries({ queryKey: ['fleet-console'] });
      void queryClient.invalidateQueries({
        queryKey: ['swarming-bots-current-tasks'],
      });
    }
    setLatest(false);
  };

  const handleConfirm = async () => {
    trackEvent('run_reserve', {
      componentName: 'run_reserve_button',
      dutCount: selectedDuts.length,
    });
    setLoading(true);
    const flags = [];
    if (latest) {
      flags.push(ScheduleReserveRequest_ReserveFlag.LATEST);
    }
    try {
      const resp = await fleetConsoleClient.ScheduleReserve(
        ScheduleReserveRequest.fromPartial({
          unitNames: dutNames,
          comment: comment,
          flags: flags,
        }),
      );

      const mappedResults: ReserveResult[] = (resp.results || []).map((r) => ({
        unitName: r.unitName,
        success: !!r.taskUrl,
        redirectUrl: r.taskUrl || undefined,
        errorMessage: r.errorMessage || undefined,
      }));
      setResults(mappedResults);
      setSessionId(resp.sessionId);
    } catch (e) {
      setResults(
        selectedDuts.map((dut) => ({
          unitName: dut.name,
          success: false,
          errorMessage:
            e instanceof Error ? e.message : 'Unknown connection error',
        })),
      );
    } finally {
      setLoading(false);
    }
  };

  return (
    <>
      <Button
        color="primary"
        size="small"
        startIcon={<LockIcon />}
        onClick={initializeReserve}
        disabled={
          dutNames.length === 0 || hasPermission === null || checkingPermission
        }
      >
        Reserve
      </Button>
      <ReserveDialog
        open={open}
        sessionInfo={{
          dutNames: dutNames,
          results: results,
          sessionId: sessionId,
        }}
        handleClose={handleClose}
        handleOk={handleConfirm}
        loading={loading}
        comment={comment}
        handleCommentChange={setComment}
        latest={latest}
        handleLatestChange={setLatest}
      />
      <AdminAccessRequiredDialog
        open={adminAccessRequiredDialogOpen}
        onClose={() => setAdminAccessRequiredDialogOpen(false)}
      />
      <Snackbar
        open={!!errorMessage}
        autoHideDuration={6000}
        onClose={() => setErrorMessage(null)}
        message={errorMessage}
      />
    </>
  );
}
