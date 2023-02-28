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
import { Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle } from '@mui/material';
import { customElement } from 'lit/decorators.js';
import { computed, makeObservable, observable } from 'mobx';
import { observer } from 'mobx-react-lite';
import { useCallback } from 'react';
import { createRoot, Root } from 'react-dom/client';
import { useNavigate } from 'react-router-dom';

import '../../components/dot_spinner';
import { consumer } from '../../libs/context';
import { getBuildURLPathFromBuildData } from '../../libs/url_utils';
import { consumeStore, StoreInstance, StoreProvider, useStore } from '../../store';
import commonStyle from '../../styles/common_style.css';

export interface RetryBuildDialogProps {
  readonly open: boolean;
  readonly onClose?: () => void;
  readonly container?: HTMLDivElement;
}

export const RetryBuildDialog = observer(({ open, onClose, container }: RetryBuildDialogProps) => {
  const pageState = useStore().buildPage;
  const navigate = useNavigate();

  const handleConfirm = useCallback(async () => {
    const build = await pageState.retryBuild();
    onClose?.();
    if (build) {
      navigate(getBuildURLPathFromBuildData(build));
    }
  }, [pageState]);

  return (
    <Dialog onClose={onClose} open={open} fullWidth maxWidth="sm" container={container}>
      <DialogTitle>Retry Build</DialogTitle>
      <DialogContent>
        <DialogContentText>Note: this doesn't trigger anything else (e.g. CQ).</DialogContentText>
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
  );
});

@customElement('milo-bp-retry-build-dialog')
@consumer
export class BuildPageRetryBuildDialogElement extends MobxLitElement {
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
      key: 'milo-bp-retry-build-dialog',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <StoreProvider value={this.store}>
          <RetryBuildDialog
            open={this.open}
            onClose={() => this.dispatchEvent(new Event('close', { bubbles: false }))}
            container={this.parent}
          ></RetryBuildDialog>
        </StoreProvider>
      </CacheProvider>
    );
    return this.parent;
  }

  static styles = [commonStyle];
}
