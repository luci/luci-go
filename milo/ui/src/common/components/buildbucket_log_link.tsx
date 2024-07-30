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

import { MobxLitElement } from '@adobe/lit-mobx';
import createCache, { EmotionCache } from '@emotion/cache';
import { CacheProvider } from '@emotion/react';
import { Link } from '@mui/material';
import { customElement } from 'lit/decorators.js';
import { makeObservable, observable } from 'mobx';
import { createRoot, Root } from 'react-dom/client';

import { commonStyles } from '@/common/styles/stylesheets';
import { getLogdogRawUrl } from '@/common/tools/build_utils';
import { Log } from '@/proto/go.chromium.org/luci/buildbucket/proto/common.pb';

export interface BuildbucketLogLinkProps {
  readonly log: Log;
}

/**
 * Renders a Log object.
 */
export function BuildbucketLogLink({ log }: BuildbucketLogLinkProps) {
  const logdogRawUrl = ['stdout', 'stderr'].includes(log.name)
    ? getLogdogRawUrl(log.url)
    : null;

  return (
    <>
      <Link
        href={log.viewUrl}
        target="_blank"
        css={{ color: 'var(--default-text-color)' }}
      >
        {log.name}
      </Link>
      {logdogRawUrl && (
        <>
          {' '}
          [
          <Link
            href={logdogRawUrl}
            target="_blank"
            css={{ color: 'var(--default-text-color)' }}
          >
            raw
          </Link>
          ]
        </>
      )}
    </>
  );
}

@customElement('milo-buildbucket-log-link')
export class SearchPageElement extends MobxLitElement {
  @observable.ref log!: Log;

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
      key: 'milo-search-page',
      container: this.parent,
    });
  }

  protected render() {
    this.root.render(
      <CacheProvider value={this.cache}>
        <BuildbucketLogLink log={this.log} />
      </CacheProvider>,
    );
    return this.parent;
  }

  static styles = [commonStyles];
}
