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

import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Skeleton,
  SxProps,
  Theme,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { EditorConfiguration } from 'codemirror';
import { ReactNode, RefObject, useRef } from 'react';

import CentralizedProgress from '@/clusters/components/centralized_progress/centralized_progress';
import CodeSnippet from '@/fleet/components/code_snippet/code_snippet';
import { DEFAULT_CODE_MIRROR_CONFIG } from '@/fleet/constants/component_config';
import { useUfsClient } from '@/fleet/hooks/prpc_clients';
import { getErrorMessage } from '@/fleet/utils/errors';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { BrowserDevice } from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';
import { Machine } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine.pb';
import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';
import {
  GetMachineLSERequest,
  GetMachineRequest,
} from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/rpc/fleet.pb';

export const BrowserInventoryData = ({ device }: { device: BrowserDevice }) => {
  const editorOptions = useRef<EditorConfiguration>(DEFAULT_CODE_MIRROR_CONFIG);

  const ufsClient = useUfsClient('browser');
  const machine = useQuery({
    ...ufsClient.GetMachine.query(
      GetMachineRequest.fromPartial({
        // Prefix defined in
        // https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/infra/unifiedfleet/app/util/input.go;l=31
        name: `machines/${device.id}`,
      }),
    ),
    refetchInterval: 60000,
  });

  const isAttachedDevice = !!machine.data?.attachedDevice;

  const hostname = device.ufsLabels['hostname']?.values?.[0] ?? '';

  const machineLSE = useQuery({
    ...ufsClient.GetMachineLSE.query(
      GetMachineLSERequest.fromPartial({
        // Prefix defined in
        // https://source.chromium.org/chromium/infra/infra_superproject/+/main:infra/go/src/infra/unifiedfleet/app/util/input.go;l=39
        name: `machineLSEs/${hostname}`,
      }),
    ),
    enabled: !!hostname,
    refetchInterval: 60000,
  });

  const machineTitle = machine.isLoading ? (
    <Skeleton width="150px" />
  ) : isAttachedDevice ? (
    'Attached device machine'
  ) : (
    'Machine'
  );

  const hostTitle = machine.isLoading ? (
    <Skeleton width="150px" />
  ) : isAttachedDevice ? (
    'Attached device host'
  ) : (
    'Host'
  );

  return (
    <Box>
      <InventoryDataSection
        title={machineTitle}
        data={
          machine.data
            ? JSON.stringify(Machine.toJSON(machine.data), undefined, 2)
            : ''
        }
        isLoading={machine.isLoading}
        error={machine.error}
        shivasCommand={`shivas get ${isAttachedDevice ? 'adm' : 'machine'} -json -namespace browser ${device.id}`}
        copyKind={isAttachedDevice ? 'get_adm' : 'get_machine'}
        editorOptions={editorOptions}
      />
      {hostname ? (
        <InventoryDataSection
          title={hostTitle}
          data={
            machineLSE.data
              ? JSON.stringify(MachineLSE.toJSON(machineLSE.data), undefined, 2)
              : ''
          }
          isLoading={machineLSE.isLoading}
          error={machineLSE.error}
          shivasCommand={`shivas get ${isAttachedDevice ? 'adh' : 'host'} -json -namespace browser ${hostname}`}
          copyKind={isAttachedDevice ? 'get_adh' : 'get_host'}
          editorOptions={editorOptions}
          sx={{ marginTop: '16px' }}
        />
      ) : (
        <EmptySection
          title={hostTitle}
          warning="Host information is not available (missing hostname)"
          sx={{ marginTop: '16px' }}
        />
      )}
    </Box>
  );
};

interface EmptySectionProps {
  title: ReactNode;
  warning: string;
  sx?: SxProps<Theme>;
}

const EmptySection = ({ title, warning, sx }: EmptySectionProps) => {
  return (
    <Accordion
      slotProps={{
        root: {
          sx: {
            borderRadius: '4px',
            '::before': {
              display: 'none',
            },
            ...sx,
          },
        },
      }}
      variant="outlined"
      elevation={2}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography variant="h6">{title}</Typography>
      </AccordionSummary>
      <AccordionDetails>
        <Alert severity="warning">{warning}</Alert>
      </AccordionDetails>
    </Accordion>
  );
};

interface InventoryDataSectionProps {
  title: ReactNode;
  data: string;
  isLoading: boolean;
  error: unknown;
  shivasCommand: string;
  copyKind: string;
  editorOptions: RefObject<EditorConfiguration>;
  sx?: SxProps<Theme>;
}

const InventoryDataSection = ({
  title,
  data,
  isLoading,
  error,
  shivasCommand,
  copyKind,
  editorOptions,
  sx,
}: InventoryDataSectionProps) => {
  let displayContent: ReactNode = <></>;

  if (error) {
    displayContent = (
      <Alert severity="error">
        {getErrorMessage(error, `get ${title} info from shivas`)}{' '}
      </Alert>
    );
  } else if (isLoading) {
    displayContent = <CentralizedProgress />;
  } else if (!data) {
    displayContent = (
      <Alert severity="error">{`${title} data missing from shivas`}</Alert>
    );
  } else {
    displayContent = (
      <CodeMirrorEditor value={data} initOptions={editorOptions.current} />
    );
  }

  return (
    <Accordion
      slotProps={{
        root: {
          sx: {
            borderRadius: '4px',
            '::before': {
              display: 'none',
            },
            ...sx,
          },
        },
      }}
      variant="outlined"
      elevation={2}
    >
      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
        <Typography variant="h6">{title}</Typography>
      </AccordionSummary>
      <AccordionDetails>
        <InventoryDataCodeSnippet
          displayData={displayContent}
          shivasCommand={shivasCommand}
          copyKind={copyKind}
        />
      </AccordionDetails>
    </Accordion>
  );
};

const InventoryDataCodeSnippet = ({
  displayData,
  shivasCommand,
  copyKind,
}: {
  displayData: ReactNode;
  shivasCommand: string;
  copyKind: string;
}) => {
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
          displayText={'$ ' + shivasCommand}
          copyText={shivasCommand}
          copyKind={copyKind}
        />
      </div>
      {displayData}
    </Box>
  );
};
