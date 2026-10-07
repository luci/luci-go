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

import { Delete } from '@mui/icons-material';
import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  TextField,
  Typography,
} from '@mui/material';
import { useCallback, useMemo, useState } from 'react';

import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { HealthFilterBar } from './health_filter_bar';
import { useHealthFilterState } from './use_health_filters';

export interface HealthSliceDialogProps {
  readonly open: boolean;
  readonly editingSlice: HealthSlice | null;
  readonly existingSlices: readonly HealthSlice[];
  readonly onClose: () => void;
  readonly onSave: (slice: HealthSlice) => void;
  readonly onDelete?: (sliceId: string) => void;
  readonly isSaving?: boolean;
  readonly error?: Error | null;
}

export const HealthSliceDialog = ({
  open,
  editingSlice,
  existingSlices,
  onClose,
  onSave,
  onDelete,
  isSaving = false,
  error,
}: HealthSliceDialogProps) => {
  const [title, setTitle] = useState(editingSlice?.name ?? '');
  const [filter, setFilter] = useState(editingSlice?.filter ?? '');

  const { filterCategoryDatas, isLoading: filtersLoading } =
    useHealthFilterState(open ? filter : undefined, setFilter);

  const trimmedTitle = title.trim();

  const isDuplicateTitle = useMemo(() => {
    const target = trimmedTitle.toLowerCase();
    return existingSlices.some(
      (s) =>
        s.id !== editingSlice?.id && s.name.trim().toLowerCase() === target,
    );
  }, [trimmedTitle, existingSlices, editingSlice]);

  const canSave = trimmedTitle !== '' && !isDuplicateTitle && !isSaving;

  const handleSave = useCallback(() => {
    if (!canSave) return;
    onSave(
      HealthSlice.fromPartial({
        id: editingSlice?.id ?? undefined,
        name: trimmedTitle,
        filter: filter.trim(),
      }),
    );
  }, [canSave, editingSlice, trimmedTitle, filter, onSave]);

  const handleDelete = useCallback(() => {
    if (editingSlice?.id && onDelete) {
      onDelete(editingSlice.id);
    }
  }, [editingSlice, onDelete]);

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="md"
      fullWidth
      TransitionProps={{ timeout: 0 }}
    >
      <DialogTitle sx={{ fontWeight: 'bold', pb: 1 }}>
        {editingSlice ? 'Edit Health Slice' : 'Add New Health Slice'}
      </DialogTitle>
      <DialogContent
        sx={{
          display: 'flex',
          flexDirection: 'column',
          gap: 2.5,
          pt: '16px !important',
          minHeight: 220,
        }}
      >
        <TextField
          label="Slice Title"
          required
          fullWidth
          size="small"
          placeholder="e.g. Labstations"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          error={isDuplicateTitle}
          helperText={
            isDuplicateTitle
              ? 'A slice with this name already exists.'
              : undefined
          }
          inputProps={{ 'aria-label': 'Slice Title', maxLength: 100 }}
        />

        <Box>
          <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 1 }}>
            Slice Filters (Unified Filter Bar):
          </Typography>
          <HealthFilterBar
            filterCategoryDatas={filterCategoryDatas}
            isLoading={filtersLoading}
            searchPlaceholder="Filter slice by model, pool, board..."
            disableShortcut
          />
        </Box>

        <Typography variant="caption" color="text.secondary">
          Note: Slice filters combine with any active global Filter Bar criteria
          at the top of the page.
        </Typography>

        {error && <Alert severity="error">{error.message}</Alert>}
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5 }}>
        {editingSlice?.id && onDelete && (
          <Button
            color="error"
            startIcon={<Delete />}
            onClick={handleDelete}
            disabled={isSaving}
            sx={{ mr: 'auto', textTransform: 'none' }}
          >
            Delete Slice
          </Button>
        )}
        <Button onClick={onClose} sx={{ textTransform: 'none' }}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleSave}
          disabled={!canSave}
          sx={{ textTransform: 'none', fontWeight: 'bold' }}
        >
          Save Slice
        </Button>
      </DialogActions>
    </Dialog>
  );
};
