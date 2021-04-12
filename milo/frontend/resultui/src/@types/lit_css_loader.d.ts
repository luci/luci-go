/* Copyright 2020 The LUCI Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * @fileoverview
 * Extra type definitions for lit-css-loader.
 *
 * lit-css-loader allows us to import any css file as a CSSResult. But it
 * doesn't provide a type definition file for this behavior.
 *
 * See https://github.com/bennypowers/lit-css-loader
 */

declare module '*.css' {
  import { CSSResult } from 'lit-element';
  const css: CSSResult;
  export default css;
}
