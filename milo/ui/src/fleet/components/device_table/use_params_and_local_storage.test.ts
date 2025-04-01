// Copyright 2025 The LUCI Authors.
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

import {
  getInitialValue,
  getNewStates,
  synchSearchParamToLocalStorage,
} from './use_params_and_local_storage';

describe('useParamsAndLocalStorage', () => {
  describe('getInitialValue', () => {
    it('Should priorities search parameters', () => {
      const out = getInitialValue(
        new URLSearchParams([
          ['c', 'col1'],
          ['c', 'col2'],
          ['c', 'col3'],
        ]),
        'c',
        ['col8', 'col9'],
        ['default'],
      );

      expect(out).toEqual(['col1', 'col2', 'col3']);
    });
    it('Should priorities local storage after search parameters', () => {
      const out = getInitialValue(
        new URLSearchParams([]),
        'c',
        ['col8', 'col9'],
        ['default'],
      );

      expect(out).toEqual(['col8', 'col9']);
    });
    it('Should fall back to the default values', () => {
      const out1 = getInitialValue(new URLSearchParams([]), 'c', undefined, [
        'default',
      ]);

      expect(out1).toEqual(['default']);

      const out2 = getInitialValue(
        new URLSearchParams([]),
        'c',
        [],
        ['default'],
      );

      expect(out2).toEqual(['default']);
    });
  });

  describe('synchSearchParamToLocalStorage', () => {
    it('Should not do anything if query params are set', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([['c', 'col1']]),
        'c',
        ['potato'],
        setSearchParams,
      );

      expect(setSearchParams).not.toHaveBeenCalled();
    });
    it('Should not do anything if local storage is not set', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([['c', 'col1']]),
        'c',
        undefined,
        setSearchParams,
      );

      expect(setSearchParams).not.toHaveBeenCalled();
    });
    it('Should set query parameters if local storage is set and they are not', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([]),
        'c',
        ['potato'],
        setSearchParams,
      );

      expect(setSearchParams).toHaveBeenCalledWith(
        new URLSearchParams([['c', 'potato']]),
      );
    });
    it('Should leave other query parameters unchanged', () => {
      const setSearchParams = jest.fn();
      synchSearchParamToLocalStorage(
        new URLSearchParams([['other', 'value']]),
        'c',
        ['potato'],
        setSearchParams,
      );

      expect(setSearchParams).toHaveBeenCalledWith(
        new URLSearchParams([
          ['other', 'value'],
          ['c', 'potato'],
        ]),
      );
    });
  });

  describe('getNewStates', () => {
    it('Should set a new state', () => {
      const out = getNewStates(
        ['new', 'values'],
        new URLSearchParams([['c', 'col1']]),
        'c',
        ['default'],
      );

      expect(out).toEqual({
        localStorage: ['new', 'values'],
        searchParams: new URLSearchParams([
          ['c', 'new'],
          ['c', 'values'],
        ]),
      });
    });
    it('Should delete the state if setting to the default value', () => {
      const out = getNewStates(
        ['default'],
        new URLSearchParams([['c', 'col1']]),
        'c',
        ['default'],
      );

      expect(out).toEqual({
        localStorage: undefined,
        searchParams: new URLSearchParams([]),
      });
    });
    it('Should leave other query parameters unchanged', () => {
      const out = getNewStates(
        ['default'],
        new URLSearchParams([
          ['c', 'col1'],
          ['other', 'value'],
        ]),
        'c',
        ['default'],
      );

      expect(out).toEqual({
        localStorage: undefined,
        searchParams: new URLSearchParams([['other', 'value']]),
      });
    });
  });
});
