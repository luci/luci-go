// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/**
 * An example fetch function.
 *
 * @return {Promise} a promise that fulfills with the requested data.
 */
export const getData = async (): Promise<ExampleModel> => {
  const response = await fetch('/path/to/api');
  return await response.json();
};

/**
 * An example data model.
 */
export interface ExampleModel {
    name: string;
}
