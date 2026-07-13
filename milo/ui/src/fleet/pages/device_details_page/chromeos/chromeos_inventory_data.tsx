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
import { Alert, Box, Grid, Link, Tab, Tabs, Typography } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { EditorConfiguration } from 'codemirror';
import { useMemo, useState } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import { useFeatureFlag } from '@/common/feature_flags';
import CodeSnippet from '@/fleet/components/code_snippet/code_snippet';
import { DEFAULT_CODE_MIRROR_CONFIG } from '@/fleet/constants/component_config';
import { enableModernInventoryCards } from '@/fleet/features';
import { useUfsClient } from '@/fleet/hooks/prpc_clients';
import { extractDutLabel } from '@/fleet/utils/devices';
import { getErrorMessage } from '@/fleet/utils/errors';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc';
import { GetMachineLSERequest } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/rpc/fleet.pb';

import {
  DeviceDetailsCard,
  DeviceDetailsInfo,
} from './components/cards/DeviceDetailsCard';

export interface ChromeOSInventoryDataProps {
  device: Device;
  editable?: boolean;
}

export const ChromeOSInventoryData = ({
  device,
  editable = false,
}: ChromeOSInventoryDataProps) => {
  const showModernCards = useFeatureFlag(enableModernInventoryCards);
  const [viewMode, setViewMode] = useState<'json' | 'visual'>('json');
  const editorOptions = useMemo<EditorConfiguration>(
    () => ({
      ...DEFAULT_CODE_MIRROR_CONFIG,
      readOnly: !editable,
    }),
    [editable],
  );

  const ufsNamespace = extractDutLabel('ufs_namespace', device);
  const ufsClient = useUfsClient(ufsNamespace || 'os');
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

  const command = `shivas get dut -json ${
    ufsNamespace ? `-namespace ${ufsNamespace} ` : ''
  }${device.id}`;

  const renderVisualCards = () => {
    if (machineLse.isLoading) {
      return <CentralizedProgress />;
    }
    if (machineLse.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(machineLse.error, 'get dut info from shivas')}
        </Alert>
      );
    }
    if (!machineLse.data) {
      return <Alert severity="info">No inventory spec data available.</Alert>;
    }
    return (
      <Grid container spacing={2}>
        <Grid item xs={12} md={6}>
          <DeviceDetailsCard
            data={machineLse.data as DeviceDetailsInfo}
            editable={editable}
          />
        </Grid>
      </Grid>
    );
  };

  const renderJsonEditor = () => {
    if (machineLse.isLoading) {
      return <CentralizedProgress />;
    }
    if (machineLse.isError) {
      return (
        <Alert severity="error">
          {getErrorMessage(machineLse.error, 'get dut info from shivas')}
        </Alert>
      );
    }
    if (!machineLse.data) {
      return <Alert severity="info">No inventory spec data available.</Alert>;
    }
    return (
      <Box
        sx={{
          border: (theme) => `1px solid ${theme.palette.divider}`,
          borderRadius: (theme) => theme.shape.borderRadius,
          overflow: 'hidden',
          maxHeight: '70vh',
          display: 'flex',
          flexDirection: 'column',
        }}
      >
        <Box sx={{ overflow: 'auto', flexGrow: 1 }}>
          <CodeMirrorEditor
            value={JSON.stringify(machineLse.data, null, 2)}
            initOptions={editorOptions}
          />
        </Box>
      </Box>
    );
  };

  const activeViewMode = showModernCards ? viewMode : 'json';

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

      {showModernCards && (
        <Box
          sx={{
            borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
          }}
        >
          <Tabs
            value={viewMode}
            onChange={(_, newMode) => {
              if (newMode !== null) setViewMode(newMode);
            }}
            aria-label="view mode tabs"
          >
            <Tab
              value="json"
              label="JSON"
              icon={<CodeIcon sx={{ fontSize: '1.2rem' }} />}
              iconPosition="start"
              aria-label="json view"
            />
            <Tab
              value="visual"
              label="Visual Dashboard"
              icon={<ViewModuleIcon sx={{ fontSize: '1.2rem' }} />}
              iconPosition="start"
              aria-label="visual view"
            />
          </Tabs>
        </Box>
      )}

      {activeViewMode === 'visual' ? renderVisualCards() : renderJsonEditor()}
    </Box>
  );
};
