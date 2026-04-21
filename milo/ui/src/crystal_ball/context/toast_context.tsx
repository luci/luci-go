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

import { Snackbar } from '@mui/material';
import React, { useCallback, useState } from 'react';

import { formatApiError } from '@/crystal_ball/utils';

import { ToastContext, ToastSeverity } from './toast_context_instance';

interface ToastState {
  message: string;
  severity: ToastSeverity;
}

/**
 * Provides a centralized toast notification system for Crystal Ball.
 */
export function ToastProvider({ children }: { children: React.ReactNode }) {
  const [toastState, setToastState] = useState<ToastState>({
    message: '',
    severity: 'success',
  });

  const handleCloseToast = () => {
    setToastState((prev) => ({ ...prev, message: '' }));
  };

  const showSuccessToast = useCallback((message: string) => {
    setToastState({ message, severity: 'success' });
  }, []);

  const showWarningToast = useCallback((message: string) => {
    setToastState({ message, severity: 'warning' });
  }, []);

  const showErrorToast = useCallback((e: unknown, defaultMessage: string) => {
    setToastState({
      message: formatApiError(e, defaultMessage),
      severity: 'error',
    });
  }, []);

  return (
    <ToastContext.Provider
      value={{ showSuccessToast, showWarningToast, showErrorToast }}
    >
      {children}
      <Snackbar
        open={Boolean(toastState.message)}
        autoHideDuration={toastState.severity === 'error' ? undefined : 4000}
        onClose={handleCloseToast}
        message={toastState.message}
        slotProps={{
          content: {
            sx: {
              ...(toastState.severity === 'error' && {
                bgcolor: 'error.main',
                color: 'error.contrastText',
              }),
              ...(toastState.severity === 'warning' && {
                bgcolor: 'warning.main',
                color: 'warning.contrastText',
              }),
            },
          },
        }}
      />
    </ToastContext.Provider>
  );
}
