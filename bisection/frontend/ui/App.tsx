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


import './styles/style.css';
import * as React from 'react';
import { Route, Routes, useParams } from 'react-router-dom';


// Redirect all pages to the corresponding page under MILO.
// The LUCI bisection URL is only kept to keep legacy links working.
// All contents are moved to MILO.
export const App = () => {
  return (
    <Routes>
      <Route path='/'>
          <Route path='analysis/b/:bbid' element={<RedirectAnalysis />} />
          <Route path='*' element={<Redirect />} />
        </Route>
    </Routes>
  );
};

const getRedirectBaseURL = () => {
  const isProd = window.location.host === "luci-bisection.appspot.com"
  return isProd ? 'https://ci.chromium.org/ui/bisection': 'https://luci-milo-dev/ui/bisection'
}

const Redirect = () => {
  window.location.href = getRedirectBaseURL();
  return <></>;
};

const RedirectAnalysis = () => {
  const { bbid } = useParams();
  window.location.href = `${getRedirectBaseURL()}/analysis/b/${bbid}`;
  return <></>;
}

