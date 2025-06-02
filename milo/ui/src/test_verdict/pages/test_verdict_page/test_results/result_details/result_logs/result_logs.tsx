// Copyright 2024 The LUCI Authors.
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

import { Box, Divider } from '@mui/material';
import { memo, useRef, useState } from 'react';
import { Panel, PanelGroup, PanelResizeHandle } from 'react-resizable-panels';
import { useDebounce, useWindowSize } from 'react-use';

import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

import { TOP_PANEL_EXPANDED_ON, TOP_PANEL_EXPANDED_PARAM } from '../constants';

import { ResultLogsProvider } from './context';
import { LogTable } from './log_table';

export const ResultLogs = memo(function ResultLogs() {
  const [height, setHeight] = useState('70vh');
  const [searchParams] = useSyncedSearchParams();

  const topPanelExpandedParam = searchParams.get(TOP_PANEL_EXPANDED_PARAM);
  const { height: windowHeight } = useWindowSize();
  const boxRef = useRef<HTMLDivElement>(null);

  // We wait for the transition effect on the top parts of the page to finish
  // and give the user time to finish resizing the window
  // before calclating the new height.
  useDebounce(
    () => {
      if (boxRef.current) {
        if (topPanelExpandedParam === TOP_PANEL_EXPANDED_ON) {
          const topDistance = boxRef.current.getBoundingClientRect().top;
          setHeight(`${windowHeight - topDistance}px`);
        } else {
          setHeight('70vh');
        }
      }
    },
    300,
    [windowHeight, topPanelExpandedParam],
  );
  return (
    <ResultLogsProvider>
      <Box
        sx={{
          height,
        }}
        ref={boxRef}
      >
        <PanelGroup direction="horizontal">
          <PanelResizeHandle>
            <Divider
              sx={{
                '&:hover,&:active': {
                  '&::before, &::after': {
                    borderColor: 'blue',
                  },
                },
              }}
              orientation="vertical"
            >
              ||
            </Divider>
          </PanelResizeHandle>
          <Panel>
            <LogTable />
          </Panel>
        </PanelGroup>
      </Box>
    </ResultLogsProvider>
  );
});
