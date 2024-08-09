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

import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { RecoverableErrorBoundary } from '@/common/components/error_handling';

export function LogDefaultTab() {
  const { pathname } = useLocation();
  const navigate = useNavigate();

  // Remove any trailing '/' so the new URL won't contain '//'.
  const basePath = pathname.replace(/\/*$/, '');
  const newUrl = `${basePath}/test-logs`;

  useEffect(
    () => navigate(newUrl, { replace: true }),
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
    // See the documentation for `<LoginPage />` for why we handle error this
    // way.
    <RecoverableErrorBoundary key="default">
      <LogDefaultTab />
    </RecoverableErrorBoundary>
  );
}
