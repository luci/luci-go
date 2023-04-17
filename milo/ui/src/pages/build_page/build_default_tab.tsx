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

import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';

import { useStore } from '../../store';

// TODO(weiweilin): add a unit test once jest is set up.
export function BuildDefaultTab() {
  const { pathname } = useLocation();
  const store = useStore();
  const navigate = useNavigate();

  // Remove any trailing '/' so the new URL won't contain '//'.
  const basePath = pathname.replace(/\/*$/, '');

  const newUrl = `${basePath}/${store.userConfig.build.defaultTab}`;
  useEffect(() => navigate(newUrl, { replace: true }));

  return <></>;
}
