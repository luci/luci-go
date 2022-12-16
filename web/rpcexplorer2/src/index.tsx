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

import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';

import createInnerHTMLSanitizingPolicy from '@chopsui/trusted-types-policy';

import App from './app';

/**
 * DO NOT DELETE
 *
 * This line initializes the HTML trusted types policy.
 * This policy makes sure that no dangerous HTML can be
 * inserted in the DOM without sanitization, reducing the risk
 * of XSS that can be introduced by your code or a library
 * that you use.
 */
createInnerHTMLSanitizingPolicy();

const container = document.getElementById('app-root');
// Below ESlint is disabled based on ReactJS recommendation
// https://reactjs.org/blog/2022/03/08/react-18-upgrade-guide.html#updates-to-client-rendering-apis
// eslint-disable-next-line @typescript-eslint/no-non-null-assertion
const root = createRoot(container!);
root.render(
    <BrowserRouter basename='/rpcexplorer'>
      <App />
    </BrowserRouter>,
);
