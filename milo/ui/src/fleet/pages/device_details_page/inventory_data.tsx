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

import { Alert, Box } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { EditorConfiguration } from 'codemirror';
import { useRef } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import CodeSnippet from '@/fleet/components/code_snippet/code_snippet';
import { DEFAULT_CODE_MIRROR_CONFIG } from '@/fleet/constants/component_config';
import { useUfsClient } from '@/fleet/hooks/prpc_clients';
import { extractDutLabel } from '@/fleet/utils/devices';
import { getErrorMessage } from '@/fleet/utils/errors';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { Device } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { GetMachineLSERequest } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/rpc/fleet.pb';

export const InventoryData = ({ device }: { device: Device }) => {
  const editorOptions = useRef<EditorConfiguration>(DEFAULT_CODE_MIRROR_CONFIG);
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

  let responseDisplay = <></>;

  if (machineLse.error) {
    responseDisplay = (
      <Alert severity="error">
        {getErrorMessage(machineLse.error, 'get dut info from shivas')}{' '}
      </Alert>
    );
  } else if (machineLse.isLoading) {
    responseDisplay = <CentralizedProgress />;
  } else if (!machineLse.data) {
    responseDisplay = (
      <Alert severity="error">{'MachineLSE data missing from shivas'}</Alert>
    );
  } else {
    responseDisplay = (
      <CodeMirrorEditor
        value={JSON.stringify(machineLse.data ?? '', undefined, 2)}
        initOptions={editorOptions.current}
      />
    );
  }

  return (
    <Box>
      <div
        css={{
          marginBottom: 16,
          display: 'flex',
          flexDirection: 'column',
          gap: '16px',
        }}
      >
        <span>
          Equivalent{' '}
          <a href="http://go/shivas" target="_blank" rel="noreferrer">
            shivas
          </a>{' '}
          command:{' '}
        </span>
        <CodeSnippet
          displayText={'$ ' + command}
          copyText={command}
          copyKind="get_dut"
        />
      </div>
      {responseDisplay}
    </Box>
  );
};
