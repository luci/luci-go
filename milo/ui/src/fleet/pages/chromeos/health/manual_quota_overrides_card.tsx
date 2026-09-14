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

import { DeleteOutline } from '@mui/icons-material';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  IconButton,
  Snackbar,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { useState } from 'react';

import { useModelQuotaOverrides } from './use_model_quota_overrides';
import { validateQuota } from './validation_utils';

const validateModel = (val: string): string => {
  if (val.trim() === '') {
    return 'Model identifier cannot be empty.';
  }
  return '';
};

export const ManualQuotaOverridesCard = () => {
  const {
    overridesQuery,
    setOverrideMutation,
    deleteOverrideMutation,
    canEdit,
    isPermissionLoading,
  } = useModelQuotaOverrides();

  const [modelInput, setModelInput] = useState<string>('');
  const [quotaInput, setQuotaInput] = useState<string>('');
  const [modelError, setModelError] = useState<string>('');
  const [quotaError, setQuotaError] = useState<string>('');
  const [snackbarMessage, setSnackbarMessage] = useState<string | null>(null);
  const [snackbarSeverity, setSnackbarSeverity] = useState<'success' | 'error'>(
    'success',
  );
  const [modelToDelete, setModelToDelete] = useState<string | null>(null);

  const overrides = overridesQuery.data?.overrides ?? [];

  const resetForm = () => {
    setModelInput('');
    setQuotaInput('');
    setModelError('');
    setQuotaError('');
  };

  const handleModelChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setModelInput(val);
    setModelError(validateModel(val));
  };

  const handleQuotaChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setQuotaInput(val);
    setQuotaError(validateQuota(val));
  };

  const handleSave = async () => {
    const mErr = validateModel(modelInput);
    const qErr = validateQuota(quotaInput);
    setModelError(mErr);
    setQuotaError(qErr);

    if (mErr || qErr) {
      return;
    }

    const trimmedModel = modelInput.trim();
    const num = Number(quotaInput);

    try {
      await setOverrideMutation.mutateAsync({
        model: trimmedModel,
        overriddenExpectedQuantity: num,
      });
      setSnackbarSeverity('success');
      setSnackbarMessage(
        `Quota override for "${trimmedModel}" saved successfully.`,
      );
      resetForm();
    } catch (err: unknown) {
      setSnackbarSeverity('error');
      const msg =
        err instanceof Error
          ? err.message
          : `Failed to save quota override for "${trimmedModel}".`;
      setSnackbarMessage(msg);
    }
  };

  const handleDelete = async (model: string) => {
    try {
      await deleteOverrideMutation.mutateAsync(model);
      setSnackbarSeverity('success');
      setSnackbarMessage(`Quota override for "${model}" removed successfully.`);
    } catch (err: unknown) {
      setSnackbarSeverity('error');
      const msg =
        err instanceof Error
          ? err.message
          : `Failed to remove quota override for "${model}".`;
      setSnackbarMessage(msg);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!isSaveDisabled) {
      handleSave();
    }
  };

  const existingOverride = overrides.find(
    (o) => o.model.toLowerCase() === modelInput.trim().toLowerCase(),
  );
  const isUnchanged =
    existingOverride !== undefined &&
    existingOverride.overriddenExpectedQuantity === Number(quotaInput);

  const isSaveDisabled =
    !canEdit ||
    !modelInput.trim() ||
    !quotaInput.trim() ||
    Boolean(modelError) ||
    Boolean(quotaError) ||
    isUnchanged ||
    setOverrideMutation.isPending;

  return (
    <Card
      variant="outlined"
      sx={{
        maxWidth: 680,
        width: '100%',
        borderRadius: 2,
      }}
    >
      <CardHeader
        title={
          <Typography sx={{ fontWeight: 'bold', fontSize: 16 }}>
            Manual Model Quota Overrides
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
        <Box display="flex" flexDirection="column" gap={0.75}>
          <Typography variant="body2" color="text.secondary">
            Define model-specific expected quota overrides to ensure models with
            unique operational realities aren&apos;t penalized.
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Note: This is a temporary component that provides working
            functionality, but will be replaced with a performance ranking later
            on.
          </Typography>
        </Box>

        {overridesQuery.isPending ? (
          <Box display="flex" justifyContent="center" py={3}>
            <CircularProgress size={24} />
          </Box>
        ) : overridesQuery.isError ? (
          <Alert severity="error">Failed to load model quota overrides.</Alert>
        ) : (
          <>
            {/* Input Form for adding or updating an override */}
            <Box
              component="form"
              onSubmit={handleSubmit}
              display="flex"
              flexDirection="column"
              gap={2}
            >
              <Box
                display="flex"
                flexWrap="wrap"
                gap={2}
                alignItems="flex-start"
              >
                <TextField
                  label="Model"
                  size="small"
                  placeholder="e.g. brya"
                  value={modelInput}
                  onChange={handleModelChange}
                  error={Boolean(modelError)}
                  helperText={
                    modelError ||
                    (!canEdit && !isPermissionLoading
                      ? 'Read-only view (FLOPs Lead permission required).'
                      : '')
                  }
                  disabled={!canEdit || setOverrideMutation.isPending}
                  slotProps={{
                    htmlInput: {
                      'aria-label': 'Override Model Input',
                    },
                  }}
                  sx={{ width: { xs: '100%', sm: 200 } }}
                />

                <TextField
                  label="Expected Quota"
                  type="number"
                  size="small"
                  placeholder="e.g. 25"
                  value={quotaInput}
                  onChange={handleQuotaChange}
                  error={Boolean(quotaError)}
                  helperText={quotaError}
                  disabled={!canEdit || setOverrideMutation.isPending}
                  slotProps={{
                    htmlInput: {
                      min: 1,
                      'aria-label': 'Override Quota Input',
                    },
                  }}
                  sx={{ width: { xs: '100%', sm: 180 } }}
                />

                {canEdit && (
                  <Tooltip
                    title={
                      isUnchanged
                        ? 'Override already set to this value'
                        : modelError || quotaError
                          ? 'Fix errors before saving'
                          : !modelInput.trim() || !quotaInput.trim()
                            ? 'Enter model and expected quota'
                            : 'Set model quota override'
                    }
                  >
                    <span>
                      <Button
                        type="submit"
                        variant="contained"
                        disabled={isSaveDisabled}
                        sx={{ height: 40, px: 3 }}
                      >
                        {setOverrideMutation.isPending ? (
                          <CircularProgress size={20} color="inherit" />
                        ) : (
                          'Set Override'
                        )}
                      </Button>
                    </span>
                  </Tooltip>
                )}
              </Box>
            </Box>

            <Divider sx={{ my: 1 }} />

            {/* Active Overrides Table */}
            <Box>
              <Typography
                variant="subtitle2"
                sx={{ fontWeight: 'bold', mb: 1.5 }}
              >
                Active Overrides
              </Typography>

              {overrides.length === 0 ? (
                <Alert severity="info">
                  No manual model quota overrides configured.
                </Alert>
              ) : (
                <TableContainer
                  sx={{
                    maxHeight: 300,
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: 1,
                  }}
                >
                  <Table
                    size="small"
                    stickyHeader
                    aria-label="Manual Quota Overrides Table"
                  >
                    <TableHead>
                      <TableRow>
                        <TableCell sx={{ fontWeight: 'bold' }}>Model</TableCell>
                        <TableCell sx={{ fontWeight: 'bold' }}>
                          Expected Quota
                        </TableCell>
                        {canEdit && (
                          <TableCell align="right" sx={{ fontWeight: 'bold' }}>
                            Actions
                          </TableCell>
                        )}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {overrides.map((item) => (
                        <TableRow key={item.id} hover>
                          <TableCell>{item.model}</TableCell>
                          <TableCell>
                            {item.overriddenExpectedQuantity}
                          </TableCell>
                          {canEdit && (
                            <TableCell align="right">
                              <Tooltip
                                title={`Remove override for ${item.model}`}
                              >
                                <span>
                                  <IconButton
                                    size="small"
                                    color="error"
                                    aria-label={`Delete override for ${item.model}`}
                                    onClick={() => setModelToDelete(item.model)}
                                    disabled={deleteOverrideMutation.isPending}
                                  >
                                    <DeleteOutline fontSize="small" />
                                  </IconButton>
                                </span>
                              </Tooltip>
                            </TableCell>
                          )}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </Box>
          </>
        )}
      </CardContent>

      <Dialog
        open={Boolean(modelToDelete)}
        onClose={() => setModelToDelete(null)}
        aria-labelledby="delete-override-dialog-title"
      >
        <DialogTitle id="delete-override-dialog-title">
          Remove Quota Override
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2">
            Are you sure you want to remove the manual quota override for{' '}
            <strong>{modelToDelete}</strong>? This model will revert to the
            global default expected quota (or the enrolled device count if a
            Support Risk Incident is active for this model).
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setModelToDelete(null)}>Cancel</Button>
          <Button
            variant="contained"
            color="error"
            onClick={async () => {
              if (modelToDelete) {
                const target = modelToDelete;
                setModelToDelete(null);
                await handleDelete(target);
              }
            }}
            disabled={deleteOverrideMutation.isPending}
          >
            Remove
          </Button>
        </DialogActions>
      </Dialog>

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
