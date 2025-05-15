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

import { untracked } from 'mobx';
import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';
import { useStore } from '@/common/store';

import { INITIAL_DEFAULT_TAB, parseTab } from './common';

export function BuildDefaultTab() {
  const { pathname, search, hash } = useLocation();
  const store = useStore();
  const navigate = useNavigate();

  // Remove any trailing '/' so the new URL won't contain '//'.
  const basePath = pathname.replace(/\/*$/, '');

  // This is unnecessary since the component isn't an observer.
  // But add `untracked` just to be safe.
  const defaultTab =
    parseTab(untracked(() => store.userConfig.build.defaultTab)) ||
    INITIAL_DEFAULT_TAB;

  const newUrl = `${basePath}/${defaultTab}${search}${hash}`;
  useEffect(
    () => {
      navigate(newUrl, { replace: true });
    },
    // The react-router router implementation could trigger URL related updates
    // (e.g. search query update) before unmounting the component and
    // redirecting users to the new component.
    // This may cause infinite loops if the dependency isn't specified.
    [navigate, newUrl],
  );

  return <></>;
}

export function Component() {
  return (
    <RecoverableErrorBoundary
      // See the documentation in `<LoginPage />` to learn why we handle error
      // this way.
      key="default"
    >
      <BuildDefaultTab />
    </RecoverableErrorBoundary>
  );
}
