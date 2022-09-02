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
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, TextField } from '@mui/material';
import { customElement } from 'lit-element';
import { computed, makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useCallback, useState } from 'react';
import { createRoot, Root } from 'react-dom/client';

import '../../components/dot_spinner';
import { consumer } from '../../libs/context';
import { consumeStore, StoreInstance, StoreProvider, useStore } from '../../store';
import commonStyle from '../../styles/common_style.css';
import { globalStyleCache } from '../../styles/global_cache';

export interface CancelBuildDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
}

export const CancelBuildDialog = observer(({ open, onClose }: CancelBuildDialogProps) => {
  const pageState = useStore().buildPage;
  const [reason, setReason] = useState('');
  const [showError, setShowErr] = useState(false);

  const handleConfirm = useCallback(() => {
    if (!reason) {
      setShowErr(true);
      return;
    }
    pageState.cancelBuild(reason);
    onClose?.();
  }, [reason, pageState]);

  const handleUpdate = (newReason: string) => {
    setReason(newReason);
    setShowErr(false);
  };

  return (
    <CacheProvider value={globalStyleCache}>
      <Dialog onClose={onClose} open={open} fullWidth maxWidth="sm">
        <DialogTitle>Cancel Build</DialogTitle>
        <DialogContent>
          <TextField
            label="Reason"
            value={reason}
            error={showError}
            helperText={showError ? 'Reason is required' : ''}
            onChange={(e) => handleUpdate(e.target.value)}
            autoFocus
            required
            margin="dense"
            fullWidth
            multiline
            minRows={4}
            maxRows={10}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose} variant="text">
            Dismiss
          </Button>
          <Button onClick={handleConfirm} variant="contained">
            Confirm
          </Button>
        </DialogActions>
      </Dialog>
    </CacheProvider>
  );
});

@customElement('milo-bp-cancel-build-dialog')
@consumer
export class BuildPageCancelBuildDialogElement extends MobxLitElement {
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
      key: 'milo-bp-cancel-build-dialog',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider value={this.store}>
          <CancelBuildDialog
            open={this.open}
            onClose={() => this.dispatchEvent(new Event('close', { bubbles: false }))}
          ></CancelBuildDialog>
        </StoreProvider>
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
