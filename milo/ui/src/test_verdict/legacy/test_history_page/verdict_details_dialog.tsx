// Copyright 2023 The LUCI Authors.
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

import { Close } from '@mui/icons-material';
import { Button, Dialog, DialogContent } from '@mui/material';
import { observer } from 'mobx-react-lite';
import { useRef } from 'react';

import { useStore } from '@/common/store';
import { Hotkey } from '@/generic_libs/components/hotkey';

import {
  DetailsTable,
  TestHistoryDetailsTableElement,
} from './test_history_details_table';
import { ConfigWidget } from './test_history_details_table/config_widget';

export const VerdictDetailsDialog = observer(() => {
  const store = useStore();
  const pageState = store.testHistoryPage;
  const detailsTableRef = useRef<TestHistoryDetailsTableElement>(null);

  return (
    <Dialog
      open={Boolean(pageState.selectedGroup)}
      onClose={() => pageState.setSelectedGroup(null)}
      maxWidth={false}
      fullWidth
      disableScrollLock
      sx={{
        '& .MuiDialog-paper': {
          margin: 0,
          maxWidth: '100%',
          width: '100%',
          maxHeight: '60%',
          height: '60%',
          position: 'absolute',
          bottom: 0,
          borderRadius: 0,
        },
      }}
    >
      <DialogContent sx={{ padding: 0 }}>
        <div
          css={{
            display: 'grid',
            gridTemplateColumns: 'auto 1fr auto auto',
            gridGap: '5px',
            height: '35px',
            padding: '5px 10px 3px 10px',
            position: 'sticky',
            top: '0px',
            background: 'white',
            zIndex: '3',
          }}
        >
          <ConfigWidget css={{ padding: '4px 5px 0px' }} />
          <div>{/* GAP */}</div>
          <div title="press x to expand/collapse all entries">
            <Hotkey
              hotkey="x"
              handler={() => detailsTableRef.current?.toggleAllVariants()}
            >
              <Button
                onClick={() => detailsTableRef.current?.toggleAllVariants(true)}
              >
                Expand All
              </Button>
              <Button
                onClick={() =>
                  detailsTableRef.current?.toggleAllVariants(false)
                }
              >
                Collapse All
              </Button>
            </Hotkey>
          </div>
          <div title="press esc to close the test variant details table">
            <Hotkey
              hotkey="esc"
              handler={() => pageState.setSelectedGroup(null)}
            >
              <Close
                css={{
                  color: 'red',
                  cursor: 'pointer',
                  paddingTop: '5px',
                }}
                onClick={() => pageState.setSelectedGroup(null)}
              />
            </Hotkey>
          </div>
        </div>
        <DetailsTable
          ref={detailsTableRef}
          css={{ '--thdt-top-offset': '43px' }}
        />
      </DialogContent>
    </Dialog>
  );
});
