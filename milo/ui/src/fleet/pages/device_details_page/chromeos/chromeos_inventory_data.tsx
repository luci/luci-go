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

import CodeIcon from '@mui/icons-material/Code';
import ViewModuleIcon from '@mui/icons-material/ViewModule';
import {
  Alert,
  Box,
  Button,
  Grid,
  Link,
  Tab,
  Tabs,
  Typography,
} from '@mui/material';
import { useQuery, useQueryClient, useMutation } from '@tanstack/react-query';
import { EditorConfiguration } from 'codemirror';
import { useCallback, useMemo, useState } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { useFeatureFlag } from '@/common/feature_flags';
import CodeSnippet from '@/fleet/components/code_snippet/code_snippet';
import { DEFAULT_CODE_MIRROR_CONFIG } from '@/fleet/constants/component_config';
import {
  enableModernInventoryCards,
  enableInventoryEditing,
} from '@/fleet/features';
import {
  useUfsClient,
  useFleetConsoleClient,
} from '@/fleet/hooks/prpc_clients';
import { useFleetAnalytics } from '@/fleet/hooks/use_fleet_analytics';
import { extractDutLabel } from '@/fleet/utils/devices';
import { getErrorMessage } from '@/fleet/utils/errors';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { UpdateChromeOSDeviceRequest } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/chromeos.pb';
import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';
import { GetMachineLSERequest } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/rpc/fleet.pb';

import {
  DeviceDetailsCard,
  DeviceDetailsInfo,
} from './components/cards/DeviceDetailsCard';
import { LogicalSchedulingCard } from './components/cards/LogicalSchedulingCard';
import { PhysicalLocationCard } from './components/cards/PhysicalLocationCard';
import { RPMCard, RPMInfo } from './components/cards/RPMCard';
import {
  ServoHardwareCard,
  ServoInfo,
} from './components/cards/ServoHardwareCard';
import { SaveDiffDialog } from './components/common/SaveDiffDialog';
import { InventoryFormProvider } from './components/form/InventoryFormContext';
import {
  calculateDiff,
  translateDiffToEdits,
  updateNestedValues,
} from './utils/inventory_editing_utils';

export interface ChromeOSInventoryDataProps {
  device: Device;
}

export const ChromeOSInventoryData = ({
  device,
}: ChromeOSInventoryDataProps) => {
  const showModernCards = useFeatureFlag(enableModernInventoryCards);
  const isEditingEnabled = useFeatureFlag(enableInventoryEditing);

  const [viewMode, setViewMode] = useState<'json' | 'visual'>(
    showModernCards ? 'visual' : 'json',
  );
  const [editedLse, setEditedLse] = useState<MachineLSE | null>(null);
  const [originalLseSnapshot, setOriginalLseSnapshot] =
    useState<MachineLSE | null>(null);
  const [activeEditingCardId, setActiveEditingCardId] = useState<string | null>(
    null,
  );

  const [dialogOpen, setDialogOpen] = useState(false);
  const [saveState, setSaveState] = useState<
    'review' | 'saving' | 'success' | 'error'
  >('review');

  const queryClient = useQueryClient();
  const { trackEvent } = useFleetAnalytics();
  const ufsNamespace = extractDutLabel('ufs_namespace', device);
  const ufsClient = useUfsClient(ufsNamespace || 'os');
  const fleetConsoleClient = useFleetConsoleClient();

  const machineLse = useQuery({
    ...ufsClient.GetMachineLSE.query(
      GetMachineLSERequest.fromPartial({
        // Prefix defined in
        // https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/infra/unifiedfleet/app/util/input.go;l=39
        name: `machineLSEs/${device.id}`,
      }),
    ),
    refetchInterval: 60000,
  });

  const updateMutation = useMutation({
    mutationFn: (req: UpdateChromeOSDeviceRequest) =>
      fleetConsoleClient.UpdateChromeOSDevice(req),
    onSuccess: () => {
      setSaveState('success');
      queryClient.invalidateQueries({
        queryKey: ufsClient.GetMachineLSE.query(
          GetMachineLSERequest.fromPartial({
            name: `machineLSEs/${device.id}`,
          }),
        ).queryKey,
      });
      setEditedLse(null);
      setOriginalLseSnapshot(null);
    },
    onError: () => {
      setSaveState('error');
    },
  });

  const currentLse = editedLse || machineLse.data;

  const currentServo = useMemo<ServoInfo | null>(() => {
    if (!currentLse) return null;
    return (
      (currentLse.chromeosMachineLse?.deviceLse?.dut?.peripherals
        ?.servo as ServoInfo) || null
    );
  }, [currentLse]);

  const hasGlobalChanges = useMemo(() => {
    const base = originalLseSnapshot || machineLse.data;
    if (!base || !editedLse) return false;
    return calculateDiff(base, editedLse).length > 0;
  }, [machineLse.data, originalLseSnapshot, editedLse]);

  const handleUpdateFields = useCallback(
    (updates: Array<{ path: string | string[]; value: unknown }>) => {
      setOriginalLseSnapshot((prevSnapshot) => {
        if (!prevSnapshot && machineLse.data) {
          return JSON.parse(JSON.stringify(machineLse.data));
        }
        return prevSnapshot;
      });

      setEditedLse((prev: MachineLSE | null) => {
        const base = prev || machineLse.data;
        if (!base) return null;
        return updateNestedValues(
          base as unknown as Record<string, unknown>,
          updates,
        ) as unknown as MachineLSE;
      });
    },
    [machineLse.data],
  );

  const handleGlobalDiscard = () => {
    setEditedLse(null);
    setOriginalLseSnapshot(null);
    setActiveEditingCardId(null);
  };

  const handleGlobalSave = () => {
    setSaveState('review');
    setDialogOpen(true);
  };

  const executeSaveRequest = () => {
    const base = originalLseSnapshot || machineLse.data;
    if (!base || !editedLse) return;
    const { edits, paths } = translateDiffToEdits(base, editedLse);
    if (paths.length === 0) return;

    trackEvent('inventory_edit_submitted', {
      editedFields: paths.join(','),
      fieldCount: paths.length,
    });

    setSaveState('saving');
    updateMutation.mutate(
      UpdateChromeOSDeviceRequest.fromPartial({
        deviceId: device.id,
        edits,
        updateMask: paths,
        requestId: crypto.randomUUID(),
      }),
    );
  };

  const editorOptions = useMemo<EditorConfiguration>(
    () => ({
      ...DEFAULT_CODE_MIRROR_CONFIG,
      readOnly: true,
    }),
    [],
  );

  const lse = currentLse;
  const dut = lse?.chromeosMachineLse?.deviceLse?.dut;
  const labstation = lse?.chromeosMachineLse?.deviceLse?.labstation;
  const zone = lse?.zone;
  const rack = lse?.rack;
  const rpm = (dut?.peripherals?.rpm as RPMInfo) || null;

  const isLabstation = Boolean(labstation);
  const command = `shivas get ${isLabstation ? 'labstation' : 'dut'} -json ${
    ufsNamespace ? `-namespace ${ufsNamespace} ` : ''
  }${device.id}`;

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, mt: 2 }}>
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
        <Typography variant="body2" color="text.secondary">
          Equivalent{' '}
          <Link href="http://go/shivas" target="_blank" rel="noreferrer">
            shivas
          </Link>{' '}
          command:{' '}
        </Typography>
        <CodeSnippet
          displayText={command}
          copyText={command}
          copyKind="get_dut"
        />
      </Box>

      {machineLse.isLoading ? (
        <CentralizedProgress />
      ) : machineLse.isError ? (
        <Alert severity="error">
          Failed to load machine LSE configuration:{' '}
          {getErrorMessage(machineLse.error, 'load machine LSE')}
        </Alert>
      ) : (
        <>
          {showModernCards && (
            <Box
              sx={{
                borderBottom: 1,
                borderColor: 'divider',
                mb: 2,
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
              }}
            >
              <Tabs value={viewMode} onChange={(_, val) => setViewMode(val)}>
                <Tab
                  icon={<ViewModuleIcon />}
                  iconPosition="start"
                  label="Visual Dashboard"
                  value="visual"
                />
                <Tab
                  icon={<CodeIcon />}
                  iconPosition="start"
                  label="JSON"
                  value="json"
                />
              </Tabs>
              {isEditingEnabled && (
                <Box
                  sx={{
                    display: 'flex',
                    gap: 1.5,
                    pb: 1,
                    visibility: hasGlobalChanges ? 'visible' : 'hidden',
                  }}
                >
                  <Button
                    size="small"
                    variant="outlined"
                    color="inherit"
                    onClick={handleGlobalDiscard}
                    disabled={activeEditingCardId !== null}
                  >
                    Discard Changes
                  </Button>
                  <Button
                    size="small"
                    variant="contained"
                    color="primary"
                    onClick={handleGlobalSave}
                    disabled={activeEditingCardId !== null}
                  >
                    Save All Changes
                  </Button>
                </Box>
              )}
            </Box>
          )}

          {viewMode === 'visual' && showModernCards && (
            <InventoryFormProvider
              originalLse={machineLse.data || null}
              draftLse={currentLse || null}
              updateDraftFields={handleUpdateFields}
              activeEditingCardId={activeEditingCardId}
              setActiveEditingCardId={setActiveEditingCardId}
              editable={isEditingEnabled}
            >
              <Grid container spacing={2}>
                <Grid item xs={12} md={6}>
                  <DeviceDetailsCard
                    data={currentLse as DeviceDetailsInfo}
                    editable={false}
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <LogicalSchedulingCard />
                </Grid>
                <Grid item xs={12} md={6}>
                  <PhysicalLocationCard
                    zone={zone}
                    rack={rack}
                    editable={false}
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <ServoHardwareCard servo={currentServo} editable={false} />
                </Grid>
                <Grid item xs={12} md={6}>
                  <RPMCard rpm={rpm} editable={false} />
                </Grid>
              </Grid>
            </InventoryFormProvider>
          )}

          {viewMode === 'json' && (
            <Box
              sx={{
                border: '1px solid #ddd',
                borderRadius: 1,
                overflow: 'hidden',
              }}
            >
              <CodeMirrorEditor
                value={JSON.stringify(currentLse || {}, null, 2)}
                initOptions={editorOptions}
              />
            </Box>
          )}
        </>
      )}

      <SaveDiffDialog
        open={dialogOpen}
        saveState={saveState}
        diffs={calculateDiff(originalLseSnapshot || machineLse.data, editedLse)}
        onConfirm={executeSaveRequest}
        onCancel={() => setDialogOpen(false)}
        onClose={() => setDialogOpen(false)}
        errorMessage={
          updateMutation.error
            ? getErrorMessage(updateMutation.error, 'save')
            : null
        }
      />
    </Box>
  );
};
