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

import { Add } from '@mui/icons-material';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Chip,
  CircularProgress,
  Divider,
  Tooltip,
  Typography,
} from '@mui/material';
import { useCallback, useMemo, useState } from 'react';

import { FilterCategory } from '@/fleet/components/filters/use_filters';
import { HealthSlice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';

import { HealthSliceDialog } from './health_slice_dialog';
import { HealthSliceItem } from './health_slice_item';
import { useHealthSlices } from './use_health_slices';

export interface HealthSlicesCardProps {
  /** The active AIP-160 filter expression from the global Filter Bar. */
  readonly globalFilter?: string;
  /** Active filter categories from the global filter bar, used to count active global filters. */
  readonly globalFilterValues?: Record<string, FilterCategory>;
  /** Callback fired when a slice is clicked to filter the entire dashboard. */
  readonly onSelectSlice?: (slice: HealthSlice) => void;
}

type SliceDialogState =
  | { mode: 'create' }
  | { mode: 'edit'; slice: HealthSlice }
  | null;

export const HealthSlicesCard = ({
  globalFilter,
  globalFilterValues,
  onSelectSlice,
}: HealthSlicesCardProps) => {
  const {
    slicesQuery,
    createSliceMutation,
    updateSliceMutation,
    deleteSliceMutation,
  } = useHealthSlices();
  const slices = useMemo(
    () => slicesQuery.data?.healthSlices ?? [],
    [slicesQuery.data],
  );

  const [dialogState, setDialogState] = useState<SliceDialogState>(null);

  const activeGlobalFiltersCount = useMemo(() => {
    if (globalFilterValues) {
      return Object.values(globalFilterValues).filter((f) => f.isActive())
        .length;
    }
    return globalFilter?.trim() ? 1 : 0;
  }, [globalFilterValues, globalFilter]);

  const closeDialog = useCallback(() => {
    setDialogState(null);
    createSliceMutation.reset();
    updateSliceMutation.reset();
    deleteSliceMutation.reset();
  }, [createSliceMutation, updateSliceMutation, deleteSliceMutation]);

  const handleOpenAddSlice = useCallback(() => {
    setDialogState({ mode: 'create' });
  }, []);

  const handleOpenEditSlice = useCallback((sliceToEdit: HealthSlice) => {
    setDialogState({ mode: 'edit', slice: sliceToEdit });
  }, []);

  const isSaving =
    createSliceMutation.isPending ||
    updateSliceMutation.isPending ||
    deleteSliceMutation.isPending;

  const mutationError =
    createSliceMutation.error ??
    updateSliceMutation.error ??
    deleteSliceMutation.error;

  const handleSaveSlice = useCallback(
    (slice: HealthSlice) => {
      const isEditing = dialogState?.mode === 'edit';
      const mutation = isEditing ? updateSliceMutation : createSliceMutation;
      mutation.mutate(slice, { onSuccess: closeDialog });
    },
    [dialogState, createSliceMutation, updateSliceMutation, closeDialog],
  );

  const handleDeleteSlice = useCallback(
    (sliceIdToDelete: string) => {
      deleteSliceMutation.mutate(sliceIdToDelete, { onSuccess: closeDialog });
    },
    [deleteSliceMutation, closeDialog],
  );

  return (
    <Card
      variant="outlined"
      data-testid="health-slices-card"
      sx={{
        display: 'flex',
        flexDirection: 'column',
        flexGrow: 1,
        height: '100%',
        minHeight: 0,
        bgcolor: '#ffffff',
        boxShadow: '0 1px 3px rgba(0,0,0,0.05)',
      }}
    >
      <CardHeader
        title={
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              width: '100%',
              flexWrap: 'wrap',
              gap: 1,
            }}
          >
            <Typography
              component="h2"
              sx={{ fontWeight: 'bold', fontSize: 15 }}
            >
              Health Slices
            </Typography>
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
              }}
            >
              {activeGlobalFiltersCount > 0 && (
                <Tooltip title="Slice metrics are combined with active global Filter Bar criteria">
                  <Chip
                    size="small"
                    color="primary"
                    variant="outlined"
                    label={`+ ${activeGlobalFiltersCount} Global`}
                    sx={{
                      height: 20,
                      fontSize: 10,
                      fontWeight: 'bold',
                    }}
                  />
                </Tooltip>
              )}
              <Button
                size="small"
                variant="outlined"
                startIcon={<Add />}
                onClick={handleOpenAddSlice}
                sx={{
                  textTransform: 'none',
                  fontSize: 11,
                  py: 0.2,
                  px: 1,
                }}
              >
                Add Slice
              </Button>
            </Box>
          </Box>
        }
      />
      <Divider />
      <CardContent
        sx={{
          p: 2,
          flexGrow: 1,
          display: 'flex',
          flexDirection: 'column',
          gap: 1.5,
          minHeight: 0,
          overflowY: 'auto',
          scrollbarWidth: 'thin',
          '&::-webkit-scrollbar': { width: '6px' },
          '&::-webkit-scrollbar-thumb': {
            backgroundColor: 'divider',
            borderRadius: '4px',
          },
        }}
      >
        {slicesQuery.isPending ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress size={24} />
          </Box>
        ) : slicesQuery.isError ? (
          <Alert severity="error">
            Failed to load health slices: {slicesQuery.error.message}
          </Alert>
        ) : slices.length === 0 ? (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              py: 4,
              px: 2,
              textAlign: 'center',
              gap: 1,
            }}
          >
            <Typography variant="body2" color="text.secondary">
              No health slices configured.
            </Typography>
            <Button
              size="small"
              variant="outlined"
              startIcon={<Add />}
              onClick={handleOpenAddSlice}
              sx={{
                textTransform: 'none',
                fontSize: 12,
              }}
            >
              Add your first slice
            </Button>
          </Box>
        ) : (
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns:
                slices.length > 1
                  ? 'repeat(auto-fit, minmax(240px, 1fr))'
                  : '1fr',
              gap: 1.5,
            }}
          >
            {slices.map((slice) => (
              <HealthSliceItem
                key={slice.id}
                slice={slice}
                globalFilter={globalFilter}
                onSelect={onSelectSlice}
                onEdit={handleOpenEditSlice}
              />
            ))}
          </Box>
        )}
      </CardContent>

      {dialogState && (
        <HealthSliceDialog
          open={true}
          editingSlice={dialogState.mode === 'edit' ? dialogState.slice : null}
          existingSlices={slices}
          onClose={closeDialog}
          onSave={handleSaveSlice}
          onDelete={handleDeleteSlice}
          isSaving={isSaving}
          error={mutationError}
        />
      )}
    </Card>
  );
};
