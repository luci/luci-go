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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  CircularProgress,
  Divider,
  Snackbar,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { useEffect, useState } from 'react';

import { useDefaultQuota } from './use_default_quota';

const MAX_SAFE_INT32 = 2_147_483_647;

const isNotFoundError = (err: unknown): boolean => {
  if (!err) {
    return false;
  }
  if (err instanceof GrpcError && err.code === RpcCode.NOT_FOUND) {
    return true;
  }
  if (
    typeof err === 'object' &&
    err !== null &&
    'code' in err &&
    (err as { code: unknown }).code === RpcCode.NOT_FOUND
  ) {
    return true;
  }
  return false;
};

const validateQuota = (val: string): string => {
  if (val.trim() === '') {
    return 'Expected quota must be a positive whole integer greater than 0.';
  }
  const num = Number(val);
  if (isNaN(num) || num <= 0 || !Number.isInteger(num)) {
    return 'Expected quota must be a positive whole integer greater than 0.';
  }
  if (num > MAX_SAFE_INT32) {
    return `Expected quota cannot exceed ${MAX_SAFE_INT32.toLocaleString()}.`;
  }
  return '';
};

export const ExpectedQuotaCard = () => {
  const { quotaQuery, setQuotaMutation, canEdit, isPermissionLoading } =
    useDefaultQuota();
  const currentQuota = quotaQuery.data?.defaultQuota;
  const isNotFound = isNotFoundError(quotaQuery.error);

  const [inputValue, setInputValue] = useState<string>(
    currentQuota !== undefined ? String(currentQuota) : '',
  );
  const [errorText, setErrorText] = useState<string>('');
  const [snackbarMessage, setSnackbarMessage] = useState<string | null>(null);
  const [snackbarSeverity, setSnackbarSeverity] = useState<'success' | 'error'>(
    'success',
  );

  useEffect(() => {
    if (currentQuota !== undefined) {
      setInputValue(String(currentQuota));
    }
  }, [currentQuota]);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setInputValue(val);
    setErrorText(validateQuota(val));
  };

  const handleSave = async () => {
    const err = validateQuota(inputValue);
    if (err) {
      setErrorText(err);
      return;
    }

    const num = Number(inputValue);
    try {
      await setQuotaMutation.mutateAsync(num);
      setSnackbarSeverity('success');
      setSnackbarMessage('Default quota updated successfully.');
    } catch (fetchErr: unknown) {
      setSnackbarSeverity('error');
      const msg =
        fetchErr instanceof Error
          ? fetchErr.message
          : 'Failed to update quota.';
      setSnackbarMessage(msg);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!isSaveDisabled) {
      handleSave();
    }
  };

  const isChanged =
    currentQuota !== undefined
      ? inputValue.trim() !== '' && Number(inputValue) !== currentQuota
      : inputValue.trim() !== '' && !validateQuota(inputValue);
  const isSaveDisabled =
    !canEdit || !!errorText || !isChanged || setQuotaMutation.isPending;

  return (
    <Card
      variant="outlined"
      sx={{
        maxWidth: 560,
        borderRadius: 2,
      }}
    >
      <CardHeader
        title={
          <Typography sx={{ fontWeight: 'bold', fontSize: 16 }}>
            Global Default Expected Quota
          </Typography>
        }
      />
      <Divider />
      <CardContent
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 2.5,
          p: 3,
        }}
      >
        <Typography variant="body2" color="text.secondary">
          Global default quota defines the expected fleet count for any model
          that does not have an active Support Risk incident or manual quota
          override.
        </Typography>

        {quotaQuery.isPending ? (
          <Box
            display="flex"
            justifyContent="center"
            alignItems="center"
            py={2}
          >
            <CircularProgress size={24} />
          </Box>
        ) : quotaQuery.isError && !isNotFound ? (
          <Alert severity="error">Failed to load default quota.</Alert>
        ) : (
          <Box display="flex" flexDirection="column" gap={2}>
            {isNotFound && (
              <Alert severity="info">
                No default quota is currently configured. Enter a value below to
                set it.
              </Alert>
            )}
            <Box
              component="form"
              onSubmit={handleSubmit}
              display="flex"
              alignItems="flex-start"
              gap={2}
            >
              <TextField
                label="Default Quota"
                type="number"
                size="small"
                value={inputValue}
                onChange={handleInputChange}
                error={Boolean(errorText)}
                helperText={
                  errorText ||
                  (!canEdit && !isPermissionLoading
                    ? 'Read-only view (FLOPs Lead permission required to modify).'
                    : '')
                }
                disabled={!canEdit || setQuotaMutation.isPending}
                slotProps={{
                  htmlInput: {
                    min: 1,
                    'aria-label': 'Default Quota Input',
                  },
                }}
                sx={{ width: 220 }}
              />

              {canEdit && (
                <Tooltip
                  title={
                    !isChanged
                      ? 'No changes to save'
                      : errorText
                        ? 'Fix errors before saving'
                        : 'Save default quota'
                  }
                >
                  <span>
                    <Button
                      type="submit"
                      variant="contained"
                      disabled={isSaveDisabled}
                      sx={{ height: 40 }}
                    >
                      {setQuotaMutation.isPending ? (
                        <CircularProgress size={20} color="inherit" />
                      ) : (
                        'Save'
                      )}
                    </Button>
                  </span>
                </Tooltip>
              )}
            </Box>
          </Box>
        )}
      </CardContent>

      <Snackbar
        open={Boolean(snackbarMessage)}
        autoHideDuration={4000}
        onClose={() => setSnackbarMessage(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      >
        <Alert
          onClose={() => setSnackbarMessage(null)}
          severity={snackbarSeverity}
          variant="filled"
          sx={{ width: '100%' }}
        >
          {snackbarMessage}
        </Alert>
      </Snackbar>
    </Card>
  );
};
