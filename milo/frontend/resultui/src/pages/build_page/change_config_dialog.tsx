// Copyright 2022 The LUCI Authors.
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

import { MobxLitElement } from '@adobe/lit-mobx';
import createCache from '@emotion/cache';
import { CacheProvider, EmotionCache } from '@emotion/react';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  OutlinedInput,
  Select,
  Typography,
} from '@mui/material';
import { customElement } from 'lit-element';
import { computed, makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useEffect, useState } from 'react';
import { createRoot, Root } from 'react-dom/client';

import '../../components/dot_spinner';
import { consumer } from '../../libs/context';
import { consumeStore, StoreInstance, StoreProvider, useStore } from '../../store';

export interface ChangeConfigDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
  readonly container?: HTMLDivElement;
}

// An array of [buildTabName, buildTabLabel] tuples.
// Use an array of tuples instead of an Object to ensure order.
const TAB_NAME_LABEL_TUPLES = Object.freeze([
  ['build-overview', 'Overview'],
  ['build-test-results', 'Test Results'],
  ['build-steps', 'Steps & Logs'],
  ['build-related-builds', 'Related Builds'],
  ['build-timeline', 'Timeline'],
  ['build-blamelist', 'Blamelist'],
] as const);

export const ChangeConfigDialog = observer(({ open, onClose, container }: ChangeConfigDialogProps) => {
  const buildConfig = useStore().userConfig.build;
  const [tabName, setTabName] = useState(() => buildConfig.defaultTabName);

  // Sync the local state with the global config whenever the dialog is
  // (re-)opened. Without this
  // 1. the uncommitted config left in the last edit won't be discarded, and
  // 2. config changes due to other reason won't be reflected in the dialog,
  //    causing unintentional overwriting some configs.
  useEffect(() => {
    // No point updating the tab name when the dialog is not shown.
    if (!open) {
      return;
    }
    setTabName(buildConfig.defaultTabName);
  }, [open, buildConfig]);

  return (
    <Dialog onClose={onClose} open={open} fullWidth maxWidth="sm" container={container}>
      <DialogTitle>Settings</DialogTitle>
      <DialogContent sx={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: 1 }}>
        <Typography sx={{ display: 'flex', justifyContent: 'center', alignContent: 'center', flexDirection: 'column' }}>
          Default tab:
        </Typography>
        <Select
          value={tabName}
          onChange={(e) => setTabName(e.target.value)}
          input={<OutlinedInput size="small" />}
          MenuProps={{ disablePortal: true }}
          sx={{ width: '180px' }}
        >
          {TAB_NAME_LABEL_TUPLES.map(([tabName, label]) => (
            <MenuItem key={tabName} value={tabName}>
              {label}
            </MenuItem>
          ))}
        </Select>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} variant="text">
          Dismiss
        </Button>
        <Button
          onClick={() => {
            buildConfig.setDefaultTab(tabName);
            onClose?.();
          }}
          variant="contained"
        >
          Confirm
        </Button>
      </DialogActions>
    </Dialog>
  );
});

@customElement('milo-bp-change-config-dialog')
@consumer
export class BuildPageChangeConfigDialogElement extends MobxLitElement {
  static get properties() {
    return {
      open: { type: Boolean },
    };
  }

  @observable.ref @consumeStore() store!: StoreInstance;

  @observable.ref _open = false;
  @computed get open() {
    return this._open;
  }
  set open(newVal: boolean) {
    this._open = newVal;
  }

  private readonly cache: EmotionCache;
  private readonly parent: HTMLDivElement;
  private readonly root: Root;

  constructor() {
    super();
    makeObservable(this);
    this.parent = document.createElement('div');
    const child = document.createElement('div');
    this.root = createRoot(child);
    this.parent.appendChild(child);
    this.cache = createCache({
      key: 'milo-bp-change-config-dialog',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider value={this.store}>
          <ChangeConfigDialog
            open={this.open}
            onClose={() => this.dispatchEvent(new Event('close', { bubbles: false }))}
            container={this.parent}
          ></ChangeConfigDialog>
        </StoreProvider>
      </CacheProvider>
    );
    return this.parent;
  }
}
