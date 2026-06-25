// Copyright 2026 The LUCI Authors.
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

import { renderHook, waitFor, act } from '@testing-library/react';
import React from 'react';
import { MemoryRouter } from 'react-router';

import { BLANK_VALUE } from '@/fleet/constants/filters';
import * as ast from '@/fleet/utils/aip160/ast/ast';
import { SyncedSearchParamsProvider } from '@/generic_libs/hooks/synced_search_params';

import {
  StringListFilterCategory,
  StringListFilterCategoryBuilder,
} from './string_list_filter';
import {
  useFilters,
  FilterCategoryBuilder,
  FilterCategory,
} from './use_filters';

class MockFilterCategory implements FilterCategory {
  constructor(
    public key: string,
    public label: string,
  ) {}
  toAIP160() {
    return '';
  }
  render() {
    return null;
  }
  getChipLabel() {
    return '';
  }
  isActive() {
    return false;
  }
  clear() {}
  getChildrenSearchScore() {
    return 0;
  }
  setReRender() {}
}

describe('useFilters', () => {
  it('should fallback to normalized key match', () => {
    const mockBuilder: FilterCategoryBuilder<MockFilterCategory> = {
      isFilledIn: () => true,
      build: jest.fn((key, _reRender, _terms) => ({
        isError: false,
        value: new MockFilterCategory(key, key),
        warnings: [],
      })),
    };

    const builders = {
      build: mockBuilder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?filters=labels.%22build%22%3Avalue']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    expect(result.current).toBeTruthy();
    expect(result.current?.filterValues).toBeTruthy();
    expect(mockBuilder.build).toHaveBeenCalled();

    const buildCalls = (mockBuilder.build as jest.Mock).mock.calls;
    expect(buildCalls.length).toBeGreaterThanOrEqual(1);

    const terms = buildCalls[0][2];
    expect(terms).toBeTruthy();
    expect(terms.length).toBe(1);
    const arg = terms[0].simple.arg;
    expect(arg).toBeTruthy();
    expect(arg.kind).toBe('Comparable');
    expect((arg as ast.Comparable).member.value.value).toBe('value');
  });

  it('should not double quote values when generating AIP-160 string', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([{ label: 'v1', value: 'v1' }]);

    const builders2 = {
      model: builder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders2), { wrapper });

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());

    const category = result.current.filterValues?.[
      'model'
    ] as StringListFilterCategory;
    act(() => {
      category.setSelectedOptions(['v1']);
    });

    const generated = result.current.aip160();
    expect(generated).toEqual('(model = "v1")');
  });

  it('should fallback to getFilters for legacy URLs', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([
        { label: 'v1', value: 'v1' },
        { label: 'v2', value: 'v2' },
      ]);

    const builders = {
      model: builder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={['/?filters=model+%3D+%28%22v1%22+OR+%22v2%22%29']}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    expect(result.current).toBeTruthy();

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());
    const filters = result.current.filterValues;

    const category = filters?.['model'];
    expect(category).toBeInstanceOf(StringListFilterCategory);

    expect((category as StringListFilterCategory).getSelectedOptions()).toEqual(
      ['v1', 'v2'],
    );
  });

  it('should not update URL when filters are stable', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([
        { label: 'v1', value: 'v1' },
        { label: 'v2', value: 'v2' },
      ]);

    const builders = {
      model: builder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={['/?filters=model+%3D+%28%22v1%22+OR+%22v2%22%29']}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());

    const initialAip160 = result.current.aip160();

    result.current.aip160();

    expect(result.current.aip160()).toEqual(initialAip160);
  });

  it('should return stable AIP-160 string with deterministic category order', async () => {
    const builderHostGroup = new StringListFilterCategoryBuilder()
      .setLabel('Host Group')
      .setOptions([{ label: 'v1', value: 'v1' }]);

    const builderLabName = new StringListFilterCategoryBuilder()
      .setLabel('Lab Name')
      .setOptions([{ label: 'v2', value: 'v2' }]);

    const builders = {
      host_group: builderHostGroup,
      lab_name: builderLabName,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={[
          '/?filters=lab_name+%3D+%22v2%22+AND+host_group+%3D+%22v1%22',
        ]}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());

    const generated = result.current.aip160();
    expect(generated).toEqual('(host_group = "v1") AND (lab_name = "v2")');
  });

  it('should return syntax error as warning', () => {
    const builders = {};

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?filters=invalid%20aip160']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    expect(result.current).toBeTruthy();
    expect(result.current?.warnings).toBeTruthy();
  });

  it('should parse filters with parentheses and AND correctly', () => {
    const mockBuilder = {
      isFilledIn: () => true,
      build: jest.fn((key, _reRender, _terms) => ({
        isError: false as const,
        value: new MockFilterCategory(key, key),
        warnings: [],
      })),
    };

    const builders = {
      'labels."label-pool"': mockBuilder,
      'labels."ufs_zone"': mockBuilder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter
        initialEntries={[
          '/?filters=' +
            encodeURIComponent(
              '(labels."label-pool" = "cellular" AND labels."ufs_zone" = "ZONE_CHROMEOS7")',
            ),
        ]}
      >
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    expect(result.current).toBeTruthy();
    expect(result.current?.warnings).toEqual([]);
  });

  it('should handle unquoted keys in URL matching quoted builder keys', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([{ label: 'v1', value: 'v1' }]);

    const builders = {
      'ufs."model"': builder,
    };

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?filters=ufs.model%3D%22v1%22']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());

    const category = result.current.filterValues?.['ufs."model"'];
    expect(category).toBeInstanceOf(StringListFilterCategory);
    expect((category as StringListFilterCategory).getSelectedOptions()).toEqual(
      ['v1'],
    );
  });

  it('should parse both quoted and unquoted values from URL correctly', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([{ label: 'v1', value: 'v1' }]);

    const builders = {
      model: builder,
    };

    const wrapperQuoted = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?filters=model%3D%22v1%22']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result: resultQuoted } = renderHook(() => useFilters(builders), {
      wrapper: wrapperQuoted,
    });
    await waitFor(() => expect(resultQuoted.current.filterValues).toBeTruthy());
    const categoryQuoted = resultQuoted.current.filterValues?.['model'];
    expect(categoryQuoted).toBeInstanceOf(StringListFilterCategory);
    expect(
      (categoryQuoted as StringListFilterCategory).getSelectedOptions(),
    ).toEqual(['v1']);

    const wrapperUnquoted = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/?filters=model%3Dv1']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result: resultUnquoted } = renderHook(() => useFilters(builders), {
      wrapper: wrapperUnquoted,
    });
    await waitFor(() =>
      expect(resultUnquoted.current.filterValues).toBeTruthy(),
    );
    const categoryUnquoted = resultUnquoted.current.filterValues?.['model'];
    expect(categoryUnquoted).toBeInstanceOf(StringListFilterCategory);
    expect(
      (categoryUnquoted as StringListFilterCategory).getSelectedOptions(),
    ).toEqual(['v1']);
  });

  it('should call onFilterChange when setFiltersBatch is called', async () => {
    const builder = new StringListFilterCategoryBuilder()
      .setLabel('Model')
      .setOptions([{ label: 'v1', value: 'v1' }]);

    const builders = {
      model: builder,
    };

    const onFilterChangeMock = jest.fn();

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={['/']}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(
      () => useFilters(builders, { onFilterChange: onFilterChangeMock }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());

    act(() => {
      result.current.setFiltersBatch({ model: ['v1'] });
    });

    expect(onFilterChangeMock).toHaveBeenCalled();
  });

  const testFilterParsing = async (
    filterString: string,
    expectedFilters: Record<string, string[]>,
  ) => {
    // we are just testing the parsing here so we assume all the values exists
    const builders: Record<string, FilterCategoryBuilder<FilterCategory>> = {};
    for (const key of Object.keys(expectedFilters)) {
      builders[key] = new StringListFilterCategoryBuilder()
        .setLabel(key)
        .setOptions([
          ...expectedFilters[key]
            .filter((v) => v !== '(Blank)')
            .map((v) => ({ label: v, value: v })),
          { label: 'Blank', value: BLANK_VALUE },
        ]);
    }

    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <MemoryRouter initialEntries={[`/?filters=${filterString}`]}>
        <SyncedSearchParamsProvider>{children}</SyncedSearchParamsProvider>
      </MemoryRouter>
    );

    const { result } = renderHook(() => useFilters(builders), { wrapper });

    await waitFor(() => expect(result.current.filterValues).toBeTruthy());
    const filters = result.current.filterValues;

    for (const key of Object.keys(expectedFilters)) {
      const category = filters?.[key];
      expect(category).toBeInstanceOf(StringListFilterCategory);
      expect(
        (category as StringListFilterCategory).getSelectedOptions(),
      ).toEqual(expectedFilters[key]);
    }
  };

  // These are some examples of filters Flops is actually using.
  // These tests only check for parsing regressions and cannot validate wrong filter category definitions IE adding/removing quotes.
  // Some of the run targets have been redacted since they are confidential to google
  describe('golden examples', () => {
    // eslint-disable-next-line jest/expect-expect
    it('should handle full golden example from go/fuic-wear', async () => {
      const filterString =
        // eslint-disable-next-line max-len
        'host_group+%3D+%28%22wear%3Amh%22+OR+%22wear%3Aate-cw-standalone-postsubmit%22+OR+%22wear%3Aate-cw-standalone-vts%22+OR+%22wear%3Aate-presubmit-cw-standalone%22+OR+%22wear%3Aate-wte-standalone-cts%22+OR+%22wear%3Aaurora_platform_syshealth%22+OR+%22wear%3Aaurora_power_platform_box%22+OR+%22wear%3Abtrefdevice_pool%22+OR+%22wear%3Aeos_1p_wear_engprod%22+OR+%22wear%3Aeos_ff_calibrated_unpaired_airplane_aod_on%22+OR+%22wear%3Aeos_platform_earlysignal%22+OR+%22wear%3Aeos_platform_postsubmit%22+OR+%22wear%3Aeos_platform_syshealth%22+OR+%22wear%3Aeos_power_platform_ff%22+OR+%22wear%3Aeos_power_platform_ff_sa%22+OR+%22wear%3Aeos_power_platform_ff_sa_presubmit%22+OR+%22wear%3Agw7_platform_wear_engprod%22+OR+%22wear%3Ahelios_platform_syshealth%22+OR+%22wear%3Ahelios_power_platform_ff%22+OR+%22wear%3Ahightouchlab_wear_dou%22+OR+%22wear%3Akenari_1p_wear_engprod%22+OR+%22wear%3Akenari_btwifi_platform_syshealth%22+OR+%22wear%3Akenari_platform_syshealth%22+OR+%22wear%3Aluna_1p_wear_engprod%22+OR+%22wear%3Aluna_platform_postsubmit%22+OR+%22wear%3Aluna_platform_syshealth%22+OR+%22wear%3Aluna_power_platform_box%22+OR+%22wear%3Amenari_1p_wear_engprod%22+OR+%22wear%3Amenari_power_platform_box%22+OR+%22wear%3Ameridian_1p_wear_engprod%22+OR+%22wear%3Ameridian_btwifi_platform_syshealth%22+OR+%22wear%3Ameridian_lte_platform_syshealth%22+OR+%22wear%3Ap11_platform_postsubmit%22+OR+%22wear%3Ap11_platform_syshealth%22+OR+%22wear%3Apinot_platform_wear_engprod%22+OR+%22wear%3Apower_paired_device%22+OR+%22wear%3Aqcom6100_platform_wear_engprod%22+OR+%22wear%3Ar11_1p_wear_engprod%22+OR+%22wear%3Ar11_btwifi_platform_syshealth%22+OR+%22wear%3Ar11_lab_staging%22+OR+%22wear%3Ar11_platform_postsubmit%22+OR+%22wear%3Ariesling_power_platform_ff%22+OR+%22wear%3Aselene_1p_wear_engprod%22+OR+%22wear%3Aselene_pixelbuds_box%22+OR+%22wear%3Aselene_platform_postsubmit%22+OR+%22wear%3Aselene_platform_syshealth%22+OR+%22wear%3Aselene_power_platform_box%22+OR+%22wear%3Aselene_power_platform_ff_time_check%22+OR+%22wear%3Aselene_power_platform_ff_ttw%22+OR+%22wear%3Aselene_power_platform_ff_ui_nav%22+OR+%22wear%3Aselene_power_platform_kids_ff%22+OR+%22wear%3Aselene_ytm_box%22+OR+%22wear%3Aseluna_platform_postsubmit%22+OR+%22wear%3Aseluna_power_io25_ff%22+OR+%22wear%3Aseluna_power_sysui_ff%22+OR+%22wear%3Ashared_host%22+OR+%22wear%3Asol_platform_syshealth%22+OR+%22wear%3Aw26_platform_syshealth%22+OR+%22wear%3Awatchfe_power_platform_ff%22+OR+%22wear%3Awear_connectivity_lab%22+OR+%22wear%3Awear_shared%22+OR+%22wear%3AXR-TM3%22%29';

      await testFilterParsing(filterString, {
        host_group: [
          'wear:mh',
          'wear:ate-cw-standalone-postsubmit',
          'wear:ate-cw-standalone-vts',
          'wear:ate-presubmit-cw-standalone',
          'wear:ate-wte-standalone-cts',
          'wear:aurora_platform_syshealth',
          'wear:aurora_power_platform_box',
          'wear:btrefdevice_pool',
          'wear:eos_1p_wear_engprod',
          'wear:eos_ff_calibrated_unpaired_airplane_aod_on',
          'wear:eos_platform_earlysignal',
          'wear:eos_platform_postsubmit',
          'wear:eos_platform_syshealth',
          'wear:eos_power_platform_ff',
          'wear:eos_power_platform_ff_sa',
          'wear:eos_power_platform_ff_sa_presubmit',
          'wear:gw7_platform_wear_engprod',
          'wear:helios_platform_syshealth',
          'wear:helios_power_platform_ff',
          'wear:hightouchlab_wear_dou',
          'wear:kenari_1p_wear_engprod',
          'wear:kenari_btwifi_platform_syshealth',
          'wear:kenari_platform_syshealth',
          'wear:luna_1p_wear_engprod',
          'wear:luna_platform_postsubmit',
          'wear:luna_platform_syshealth',
          'wear:luna_power_platform_box',
          'wear:menari_1p_wear_engprod',
          'wear:menari_power_platform_box',
          'wear:meridian_1p_wear_engprod',
          'wear:meridian_btwifi_platform_syshealth',
          'wear:meridian_lte_platform_syshealth',
          'wear:p11_platform_postsubmit',
          'wear:p11_platform_syshealth',
          'wear:pinot_platform_wear_engprod',
          'wear:power_paired_device',
          'wear:qcom6100_platform_wear_engprod',
          'wear:r11_1p_wear_engprod',
          'wear:r11_btwifi_platform_syshealth',
          'wear:r11_lab_staging',
          'wear:r11_platform_postsubmit',
          'wear:riesling_power_platform_ff',
          'wear:selene_1p_wear_engprod',
          'wear:selene_pixelbuds_box',
          'wear:selene_platform_postsubmit',
          'wear:selene_platform_syshealth',
          'wear:selene_power_platform_box',
          'wear:selene_power_platform_ff_time_check',
          'wear:selene_power_platform_ff_ttw',
          'wear:selene_power_platform_ff_ui_nav',
          'wear:selene_power_platform_kids_ff',
          'wear:selene_ytm_box',
          'wear:seluna_platform_postsubmit',
          'wear:seluna_power_io25_ff',
          'wear:seluna_power_sysui_ff',
          'wear:shared_host',
          'wear:sol_platform_syshealth',
          'wear:w26_platform_syshealth',
          'wear:watchfe_power_platform_ff',
          'wear:wear_connectivity_lab',
          'wear:wear_shared',
          'wear:XR-TM3',
        ],
      });
    });

    // eslint-disable-next-line jest/expect-expect
    it('should handle full golden example from go/fuic-wear-power', async () => {
      const filterString =
        // eslint-disable-next-line max-len
        'host_group+%3D+%28%22wear%3Aaurora_power_platform_box%22+OR+%22wear%3Aeos_power_platform_ff%22+OR+%22wear%3Ahelios_power_platform_ff%22+OR+%22wear%3Aluna_power_platform_box%22+OR+%22wear%3Amenari_power_platform_box%22+OR+%22wear%3Ameridian_power_platform_ff%22+OR+%22wear%3Ameridian_power_platform_ff_sa%22+OR+%22wear%3Aselene_power_platform_box%22+OR+%22wear%3Aselene_power_platform_device_ui_nav%22+OR+%22wear%3Aselene_power_platform_ff_time_check%22+OR+%22wear%3Aselene_power_platform_ff_ui_nav%22+OR+%22wear%3Aselene_power_platform_ff_ttw%22+OR+%22wear%3Aselene_power_platform_kids_ff%22+OR+%22wear%3Aseluna_power_io25_ff%22+OR+%22wear%3Aseluna_power_sysui_ff%22+OR+%22wear%3Awatchfe_power_platform_ff%22%29';

      await testFilterParsing(filterString, {
        host_group: [
          'wear:aurora_power_platform_box',
          'wear:eos_power_platform_ff',
          'wear:helios_power_platform_ff',
          'wear:luna_power_platform_box',
          'wear:menari_power_platform_box',
          'wear:meridian_power_platform_ff',
          'wear:meridian_power_platform_ff_sa',
          'wear:selene_power_platform_box',
          'wear:selene_power_platform_device_ui_nav',
          'wear:selene_power_platform_ff_time_check',
          'wear:selene_power_platform_ff_ui_nav',
          'wear:selene_power_platform_ff_ttw',
          'wear:selene_power_platform_kids_ff',
          'wear:seluna_power_io25_ff',
          'wear:seluna_power_sysui_ff',
          'wear:watchfe_power_platform_ff',
        ],
      });
    });

    // eslint-disable-next-line jest/expect-expect
    it('should handle full golden example from go/atc-fc-1', async () => {
      const filterString =
        // eslint-disable-next-line max-len
        '%28NOT+host_group%3A*+OR+%28host_group+%3D+%22acs_beto%22+OR+host_group+%3D+%22acs%3Aap%22+OR+host_group+%3D+%22acs%3Aate-atv-experimental%22+OR+host_group+%3D+%22acs%3Abeto%22+OR+host_group+%3D+%22acs%3Abeto_11%22+OR+host_group+%3D+%22acs%3Abeto_12%22+OR+host_group+%3D+%22acs%3Acamerax%22+OR+host_group+%3D+%22acs%3Acash%22+OR+host_group+%3D+%22acs%3Agambit-agentic-tv%22+OR+host_group+%3D+%22acs%3Aiw%22+OR+host_group+%3D+%22acs%3Amedia-stablewifi%22+OR+host_group+%3D+%22acs%3Apixcam-gca%22+OR+host_group+%3D+%22acs%3Asf%22+OR+host_group+%3D+%22atc%3Aaap%22+OR+host_group+%3D+%22atc%3Aabc%22+OR+host_group+%3D+%22atc%3Aadapt%22+OR+host_group+%3D+%22atc%3Aadb%22+OR+host_group+%3D+%22atc%3Aassistant-mh%22+OR+host_group+%3D+%22atc%3Aate-atv-camera-tf-test%22+OR+host_group+%3D+%22atc%3Aate-dockerized-tf-auto-experimental%22+OR+host_group+%3D+%22atc%3Aate-main%22+OR+host_group+%3D+%22atc%3Aate-main-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-mainline%22+OR+host_group+%3D+%22atc%3Aate-mainline-sim%22+OR+host_group+%3D+%22atc%3Aate-mokey%22+OR+host_group+%3D+%22atc%3Aate-mokey-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-tv-cec%22+OR+host_group+%3D+%22atc%3Aate-tv-experimental%22+OR+host_group+%3D+%22atc%3Aate-tv-keystone%22+OR+host_group+%3D+%22atc%3Aate-tv-lowram%22+OR+host_group+%3D+%22atc%3Aate-tv-stable%22+OR+host_group+%3D+%22atc%3Aate-tv-unstable%22+OR+host_group+%3D+%22atc%3Aate-wembley%22+OR+host_group+%3D+%22atc%3Aate-wembley-presubmit%22+OR+host_group+%3D+%22atc%3Aate-xr%22+OR+host_group+%3D+%22atc%3Aauto%22+OR+host_group+%3D+%22atc%3Aauto-geo%22+OR+host_group+%3D+%22atc%3Aauto-mh%22+OR+host_group+%3D+%22atc%3Acamera%22+OR+host_group+%3D+%22atc%3Acash%22+OR+host_group+%3D+%22atc%3Acrystalball%22+OR+host_group+%3D+%22atc%3Acrystalball-copper-cage%22+OR+host_group+%3D+%22atc%3Acrystalball-presubmit%22+OR+host_group+%3D+%22atc%3Acts%22+OR+host_group+%3D+%22atc%3Adecommission%22+OR+host_group+%3D+%22atc%3Adual-sim%22+OR+host_group+%3D+%22atc%3Ahaiku-mh%22+OR+host_group+%3D+%22atc%3Ahawkeye-mh%22+OR+host_group+%3D+%22atc%3Amedia%22+OR+host_group+%3D+%22atc%3Amh-androidtv%22+OR+host_group+%3D+%22atc%3Amh-haiku%22+OR+host_group+%3D+%22atc%3Aperf%22+OR+host_group+%3D+%22atc%3Apixcam-gca%22+OR+host_group+%3D+%22atc%3Aplatinum%22+OR+host_group+%3D+%22atc%3Aplatinum-wifi%22+OR+host_group+%3D+%22atc%3Apresubmit-cw-standalone%22+OR+host_group+%3D+%22atc%3Arubidium-mh%22+OR+host_group+%3D+%22atc%3Asim%22+OR+host_group+%3D+%22atc%3Aunbundled%22+OR+host_group+%3D+%22atc%3Awear-mh%22+OR+host_group+%3D+%22atc%3AXR-1265%22+OR+host_group+%3D+%22ate-dockerized-tf-ht-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus-xts%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-xts%22+OR+host_group+%3D+%22dockerized-tf-ate-staging%22+OR+host_group+%3D+%22dockerized-tf-nugget-os%22+OR+host_group+%3D+%22lab-unattended-recovery%22+OR+host_group+%3D+%22wear-mac%22+OR+host_group+%3D+%22wear%3Amh%22%29%29+AND+%28lab_name+%3D+%22acs%22+OR+lab_name+%3D+%22atc%22%29+AND+%28run_target+%3D+%22a03su%22+OR+run_target+%3D+%22a04%22+OR+run_target+%3D+%22a04e%22+OR+run_target+%3D+%22a05m%22+OR+run_target+%3D+%22a06%22+OR+run_target+%3D+%22a12%22+OR+run_target+%3D+%22a13ve%22+OR+run_target+%3D+%22a13x%22+OR+run_target+%3D+%22a14xm%22+OR+run_target+%3D+%22a15%22+OR+run_target+%3D+%22a15x%22+OR+run_target+%3D+%22a16%22+OR+run_target+%3D+%22a22x%22+OR+run_target+%3D+%22a31%22+OR+run_target+%3D+%22a32%22+OR+run_target+%3D+%22a32x%22+OR+run_target+%3D+%22a34x%22+OR+run_target+%3D+%22a665lin%22+OR+run_target+%3D+%22adt4%22+OR+run_target+%3D+%22adt4-stablewifi%22+OR+run_target+%3D+%22akita%22+OR+run_target+%3D+%22REDACTED1%22+OR+run_target+%3D+%22REDACTED-kioxia-256gb-wechat-main%22+OR+run_target+%3D+%22REDACTED-micron-128gb%22+OR+run_target+%3D+%22REDACTED-skhynix-128gb%22+OR+run_target+%3D+%22REDACTED-skhynix-128gb-instagram-main%22+OR+run_target+%3D+%22REDACTED-skhynix-128gb-snapchat-main%22+OR+run_target+%3D+%22REDACTED-skhynix-128gb-twitter-main%22+OR+run_target+%3D+%22REDACTED-skhynix-256gb-whatsapp-main%22+OR+run_target+%3D+%22blueline%22+OR+run_target+%3D+%22blueline-stablewifi%22+OR+run_target+%3D+%22REDACTED2%22+OR+run_target+%3D+%22cheetah-camsim%22+OR+run_target+%3D+%22cheetah-kioxia-256gb%22+OR+run_target+%3D+%22cheetah-kioxia-516gb%22+OR+run_target+%3D+%22cheetah-kioxia-516gb-facebook-main%22+OR+run_target+%3D+%22cheetah-kioxia-516gb-instagram-main%22+OR+run_target+%3D+%22cheetah-kioxia-516gb-pcmark-main%22+OR+run_target+%3D+%22cheetah-kioxia-516gb-twitter-main%22+OR+run_target+%3D+%22cheetah-kioxia-516gb-wechat-main%22+OR+run_target+%3D+%22cheetah-kioxia-516gb-whatsapp-main%22+OR+run_target+%3D+%22cheetah-micron-128gb%22+OR+run_target+%3D+%22cheetah-micron-128gb-benchmarking%22+OR+run_target+%3D+%22cheetah-micron-128gb-copper-cage%22+OR+run_target+%3D+%22cheetah-micron-128gb-pcmark-throttled%22+OR+run_target+%3D+%22cheetah-micron-256gb%22+OR+run_target+%3D+%22cheetah-skhynix-128gb%22+OR+run_target+%3D+%22cubs%22+OR+run_target+%3D+%22cubs-kioxia-256gb%22+OR+run_target+%3D+%22cubs-samsung-256gb%22+OR+run_target+%3D+%22deadpool%22+OR+run_target+%3D+%22eos%22+OR+run_target+%3D+%22frankel%22+OR+run_target+%3D+%22frankel-kioxia-256gb%22+OR+run_target+%3D+%22frankel-micron-128gb%22+OR+run_target+%3D+%22frankel-samsung-128gb%22+OR+run_target+%3D+%22frankel-samsung-256gb%22+OR+run_target+%3D+%22holi%22+OR+run_target+%3D+%22k39tv1_bsp_1g_titan%22+OR+run_target+%3D+%22k39tv1_bsp_titan_hamster%22+OR+run_target+%3D+%22k65v1_64_bsp%22+OR+run_target+%3D+%22k65v1_64_bsp_titan_rat%22+OR+run_target+%3D+%22k6833v1_64%22+OR+run_target+%3D+%22k6877v1_64%22+OR+run_target+%3D+%22k6893v1_64_k419%22+OR+run_target+%3D+%22k6895v1_64%22+OR+run_target+%3D+%22k6983v1_64%22+OR+run_target+%3D+%22k6989v1_64%22+OR+run_target+%3D+%22k6991v1_64%22+OR+run_target+%3D+%22k69v1_64_k510%22+OR+run_target+%3D+%22komodo%22+OR+run_target+%3D+%22komodo-kioxia-256gb-dpm%22+OR+run_target+%3D+%22komodo-samsung-128gb%22+OR+run_target+%3D+%22komodo-samsung-128gb-pcmark-main%22+OR+run_target+%3D+%22komodo-samsung-128gb-power%22+OR+run_target+%3D+%22komodo-samsung-256gb%22+OR+run_target+%3D+%22komodo-skhynix-128gb%22+OR+run_target+%3D+%22komodo-skhynix-128gb-facebook-main%22+OR+run_target+%3D+%22komodo-skhynix-128gb-instagram-main%22+OR+run_target+%3D+%22komodo-skhynix-128gb-snapchat-main%22+OR+run_target+%3D+%22komodo-skhynix-128gb-wechat-main%22+OR+run_target+%3D+%22qc_reference_phone%22+OR+run_target+%3D+%22qc_reference_phone%3Ayt-x705f%22+OR+run_target+%3D+%22rango%22+OR+run_target+%3D+%22rango-kioxia-256gb%22+OR+run_target+%3D+%22rm6762_18540%22+OR+run_target+%3D+%22rm6769%22+OR+run_target+%3D+%22rmx3231%22+OR+run_target+%3D+%22rmx3263%22+OR+run_target+%3D+%22rmx3581%22+OR+run_target+%3D+%22sabrina%22+OR+run_target+%3D+%22sargo%22+OR+run_target+%3D+%22sargo-stablewifi%22+OR+run_target+%3D+%22sdm845%22%29&order_by=devices_offline_percentage+desc%2C+devices_offline_ratio+desc';

      await testFilterParsing(filterString, {
        host_group: [
          'acs_beto',
          'acs:ap',
          'acs:ate-atv-experimental',
          'acs:beto',
          'acs:beto_11',
          'acs:beto_12',
          'acs:camerax',
          'acs:cash',
          'acs:gambit-agentic-tv',
          'acs:iw',
          'acs:media-stablewifi',
          'acs:pixcam-gca',
          'acs:sf',
          'atc:aap',
          'atc:abc',
          'atc:adapt',
          'atc:adb',
          'atc:assistant-mh',
          'atc:ate-atv-camera-tf-test',
          'atc:ate-dockerized-tf-auto-experimental',
          'atc:ate-main',
          'atc:ate-main-high-concurrency',
          'atc:ate-mainline',
          'atc:ate-mainline-sim',
          'atc:ate-mokey',
          'atc:ate-mokey-presubmit',
          'atc:ate-presubmit',
          'atc:ate-presubmit-high-concurrency',
          'atc:ate-tv-cec',
          'atc:ate-tv-experimental',
          'atc:ate-tv-keystone',
          'atc:ate-tv-lowram',
          'atc:ate-tv-stable',
          'atc:ate-tv-unstable',
          'atc:ate-wembley',
          'atc:ate-wembley-presubmit',
          'atc:ate-xr',
          'atc:auto',
          'atc:auto-geo',
          'atc:auto-mh',
          'atc:camera',
          'atc:cash',
          'atc:crystalball',
          'atc:crystalball-copper-cage',
          'atc:crystalball-presubmit',
          'atc:cts',
          'atc:decommission',
          'atc:dual-sim',
          'atc:haiku-mh',
          'atc:hawkeye-mh',
          'atc:media',
          'atc:mh-androidtv',
          'atc:mh-haiku',
          'atc:perf',
          'atc:pixcam-gca',
          'atc:platinum',
          'atc:platinum-wifi',
          'atc:presubmit-cw-standalone',
          'atc:rubidium-mh',
          'atc:sim',
          'atc:unbundled',
          'atc:wear-mh',
          'atc:XR-1265',
          'ate-dockerized-tf-ht-tv',
          'ate-dockerized-tf-tv',
          'ate-dockerized-tf-tv-plus',
          'ate-dockerized-tf-tv-plus-xts',
          'ate-dockerized-tf-tv-xts',
          'dockerized-tf-ate-staging',
          'dockerized-tf-nugget-os',
          'lab-unattended-recovery',
          'wear-mac',
          'wear:mh',
          '(Blank)',
        ],
        lab_name: ['acs', 'atc'],
        run_target: [
          'a03su',
          'a04',
          'a04e',
          'a05m',
          'a06',
          'a12',
          'a13ve',
          'a13x',
          'a14xm',
          'a15',
          'a15x',
          'a16',
          'a22x',
          'a31',
          'a32',
          'a32x',
          'a34x',
          'a665lin',
          'adt4',
          'adt4-stablewifi',
          'akita',
          'REDACTED1',
          'REDACTED-kioxia-256gb-wechat-main',
          'REDACTED-micron-128gb',
          'REDACTED-skhynix-128gb',
          'REDACTED-skhynix-128gb-instagram-main',
          'REDACTED-skhynix-128gb-snapchat-main',
          'REDACTED-skhynix-128gb-twitter-main',
          'REDACTED-skhynix-256gb-whatsapp-main',
          'blueline',
          'blueline-stablewifi',
          'REDACTED2',
          'cheetah-camsim',
          'cheetah-kioxia-256gb',
          'cheetah-kioxia-516gb',
          'cheetah-kioxia-516gb-facebook-main',
          'cheetah-kioxia-516gb-instagram-main',
          'cheetah-kioxia-516gb-pcmark-main',
          'cheetah-kioxia-516gb-twitter-main',
          'cheetah-kioxia-516gb-wechat-main',
          'cheetah-kioxia-516gb-whatsapp-main',
          'cheetah-micron-128gb',
          'cheetah-micron-128gb-benchmarking',
          'cheetah-micron-128gb-copper-cage',
          'cheetah-micron-128gb-pcmark-throttled',
          'cheetah-micron-256gb',
          'cheetah-skhynix-128gb',
          'cubs',
          'cubs-kioxia-256gb',
          'cubs-samsung-256gb',
          'deadpool',
          'eos',
          'frankel',
          'frankel-kioxia-256gb',
          'frankel-micron-128gb',
          'frankel-samsung-128gb',
          'frankel-samsung-256gb',
          'holi',
          'k39tv1_bsp_1g_titan',
          'k39tv1_bsp_titan_hamster',
          'k65v1_64_bsp',
          'k65v1_64_bsp_titan_rat',
          'k6833v1_64',
          'k6877v1_64',
          'k6893v1_64_k419',
          'k6895v1_64',
          'k6983v1_64',
          'k6989v1_64',
          'k6991v1_64',
          'k69v1_64_k510',
          'komodo',
          'komodo-kioxia-256gb-dpm',
          'komodo-samsung-128gb',
          'komodo-samsung-128gb-pcmark-main',
          'komodo-samsung-128gb-power',
          'komodo-samsung-256gb',
          'komodo-skhynix-128gb',
          'komodo-skhynix-128gb-facebook-main',
          'komodo-skhynix-128gb-instagram-main',
          'komodo-skhynix-128gb-snapchat-main',
          'komodo-skhynix-128gb-wechat-main',
          'qc_reference_phone',
          'qc_reference_phone:yt-x705f',
          'rango',
          'rango-kioxia-256gb',
          'rm6762_18540',
          'rm6769',
          'rmx3231',
          'rmx3263',
          'rmx3581',
          'sabrina',
          'sargo',
          'sargo-stablewifi',
          'sdm845',
        ],
      });
    });

    // eslint-disable-next-line jest/expect-expect
    it('should handle full golden example from go/atc-fc-2', async () => {
      const filterString =
        // eslint-disable-next-line max-len
        '%28NOT+host_group%3A*+OR+%28host_group+%3D+%22acs_beto%22+OR+host_group+%3D+%22acs%3Aap%22+OR+host_group+%3D+%22acs%3Aate-atv-experimental%22+OR+host_group+%3D+%22acs%3Abeto%22+OR+host_group+%3D+%22acs%3Abeto_11%22+OR+host_group+%3D+%22acs%3Abeto_12%22+OR+host_group+%3D+%22acs%3Acamerax%22+OR+host_group+%3D+%22acs%3Acash%22+OR+host_group+%3D+%22acs%3Agambit-agentic-tv%22+OR+host_group+%3D+%22acs%3Aiw%22+OR+host_group+%3D+%22acs%3Amedia-stablewifi%22+OR+host_group+%3D+%22acs%3Apixcam-gca%22+OR+host_group+%3D+%22acs%3Asf%22+OR+host_group+%3D+%22atc%3Aaap%22+OR+host_group+%3D+%22atc%3Aabc%22+OR+host_group+%3D+%22atc%3Aadapt%22+OR+host_group+%3D+%22atc%3Aadb%22+OR+host_group+%3D+%22atc%3Aassistant-mh%22+OR+host_group+%3D+%22atc%3Aate-atv-camera-tf-test%22+OR+host_group+%3D+%22atc%3Aate-dockerized-tf-auto-experimental%22+OR+host_group+%3D+%22atc%3Aate-main%22+OR+host_group+%3D+%22atc%3Aate-main-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-mainline%22+OR+host_group+%3D+%22atc%3Aate-mainline-sim%22+OR+host_group+%3D+%22atc%3Aate-mokey%22+OR+host_group+%3D+%22atc%3Aate-mokey-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-tv-cec%22+OR+host_group+%3D+%22atc%3Aate-tv-experimental%22+OR+host_group+%3D+%22atc%3Aate-tv-keystone%22+OR+host_group+%3D+%22atc%3Aate-tv-lowram%22+OR+host_group+%3D+%22atc%3Aate-tv-stable%22+OR+host_group+%3D+%22atc%3Aate-tv-unstable%22+OR+host_group+%3D+%22atc%3Aate-wembley%22+OR+host_group+%3D+%22atc%3Aate-wembley-presubmit%22+OR+host_group+%3D+%22atc%3Aate-xr%22+OR+host_group+%3D+%22atc%3Aauto%22+OR+host_group+%3D+%22atc%3Aauto-geo%22+OR+host_group+%3D+%22atc%3Aauto-mh%22+OR+host_group+%3D+%22atc%3Acamera%22+OR+host_group+%3D+%22atc%3Acash%22+OR+host_group+%3D+%22atc%3Acrystalball%22+OR+host_group+%3D+%22atc%3Acrystalball-copper-cage%22+OR+host_group+%3D+%22atc%3Acrystalball-presubmit%22+OR+host_group+%3D+%22atc%3Acts%22+OR+host_group+%3D+%22atc%3Adecommission%22+OR+host_group+%3D+%22atc%3Adual-sim%22+OR+host_group+%3D+%22atc%3Ahaiku-mh%22+OR+host_group+%3D+%22atc%3Ahawkeye-mh%22+OR+host_group+%3D+%22atc%3Amedia%22+OR+host_group+%3D+%22atc%3Amh-androidtv%22+OR+host_group+%3D+%22atc%3Amh-haiku%22+OR+host_group+%3D+%22atc%3Aperf%22+OR+host_group+%3D+%22atc%3Apixcam-gca%22+OR+host_group+%3D+%22atc%3Aplatinum%22+OR+host_group+%3D+%22atc%3Aplatinum-wifi%22+OR+host_group+%3D+%22atc%3Apresubmit-cw-standalone%22+OR+host_group+%3D+%22atc%3Arubidium-mh%22+OR+host_group+%3D+%22atc%3Asim%22+OR+host_group+%3D+%22atc%3Aunbundled%22+OR+host_group+%3D+%22atc%3Awear-mh%22+OR+host_group+%3D+%22atc%3AXR-1265%22+OR+host_group+%3D+%22ate-dockerized-tf-ht-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus-xts%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-xts%22+OR+host_group+%3D+%22dockerized-tf-ate-staging%22+OR+host_group+%3D+%22dockerized-tf-nugget-os%22+OR+host_group+%3D+%22lab-unattended-recovery%22+OR+host_group+%3D+%22wear-mac%22+OR+host_group+%3D+%22wear%3Amh%22%29%29+AND+%28lab_name+%3D+%22acs%22+OR+lab_name+%3D+%22atc%22%29+AND+%28run_target+%3D+%22atoll%22+OR+run_target+%3D+%22aurora%22+OR+run_target+%3D+%22austin%22+OR+run_target+%3D+%22bramble%22+OR+run_target+%3D+%22bramble-stablewifi%22+OR+run_target+%3D+%22coral%22+OR+run_target+%3D+%22coral-stablewifi%22+OR+run_target+%3D+%22felix%22+OR+run_target+%3D+%22felix-kioxia-256gb%22+OR+run_target+%3D+%22felix-kioxia-256gb-facebook-main%22+OR+run_target+%3D+%22felix-kioxia-256gb-instagram-main%22+OR+run_target+%3D+%22felix-kioxia-256gb-snapchat-main%22+OR+run_target+%3D+%22felix-kioxia-256gb-wechat-main%22+OR+run_target+%3D+%22felix-micron-256gb%22+OR+run_target+%3D+%22felix-micron-256gb-twitter-main%22+OR+run_target+%3D+%22felix-micron-256gb-whatsapp-main%22+OR+run_target+%3D+%22husky%22+OR+run_target+%3D+%22husky-kioxia-256gb%22+OR+run_target+%3D+%22husky-samsung-128gb%22+OR+run_target+%3D+%22husky-samsung-128gb-copper-cage%22+OR+run_target+%3D+%22husky-samsung-128gb-dpm%22+OR+run_target+%3D+%22husky-samsung-128gb-instagram-main%22+OR+run_target+%3D+%22husky-samsung-128gb-power%22+OR+run_target+%3D+%22husky-samsung-128gb-power-whatsapp%22+OR+run_target+%3D+%22husky-samsung-128gb-snapchat-main%22+OR+run_target+%3D+%22husky-samsung-128gb-twitter-main%22+OR+run_target+%3D+%22husky-samsung-128gb-wechat-main%22+OR+run_target+%3D+%22husky-samsung-128gb-whatsapp-main%22+OR+run_target+%3D+%22husky-skhynix-128gb%22+OR+run_target+%3D+%22husky-skhynix-128gb-facebook-main%22+OR+run_target+%3D+%22husky-skhynix-128gb-facebook-pgagnostic%22+OR+run_target+%3D+%22husky-skhynix-128gb-pcmark-main%22+OR+run_target+%3D+%22husky-stablewifi%22+OR+run_target+%3D+%22kalama%22+OR+run_target+%3D+%22kodiak%22+OR+run_target+%3D+%22kodiak_2%22+OR+run_target+%3D+%22kodiak_2-micron-256gb%22+OR+run_target+%3D+%22kodiak_2-samsung-256gb%22+OR+run_target+%3D+%22kodiak-micron-1024gb%22+OR+run_target+%3D+%22kodiak-samsung-256gb%22+OR+run_target+%3D+%22kona%22+OR+run_target+%3D+%22menari_btwifi%22+OR+run_target+%3D+%22menari_lte%22+OR+run_target+%3D+%22meridian_btwifi%22+OR+run_target+%3D+%22meridian_lte%22+OR+run_target+%3D+%22merlinnfc%22+OR+run_target+%3D+%22milanf%22+OR+run_target+%3D+%22raven%22+OR+run_target+%3D+%22raven-camsim%22+OR+run_target+%3D+%22raven-kioxia-128gb%22+OR+run_target+%3D+%22raven-kioxia-128gb-facebook-main%22+OR+run_target+%3D+%22raven-kioxia-128gb-netflix-main%22+OR+run_target+%3D+%22raven-kioxia-128gb-twitter-main%22+OR+run_target+%3D+%22raven-skhynix-128gb%22+OR+run_target+%3D+%22raven-skhynix-128gb-power%22+OR+run_target+%3D+%22raven-skhynix-256gb%22+OR+run_target+%3D+%22raven-skhynix-256gb-pcmark-main%22+OR+run_target+%3D+%22raven-toshiba-256gb-wechat-main%22+OR+run_target+%3D+%22raven-toshiba-256gb-whatsapp-main%22+OR+run_target+%3D+%22sailfish%22+OR+run_target+%3D+%22sailfish-stablewifi%22+OR+run_target+%3D+%22stallion%22+OR+run_target+%3D+%22sunfish%22+OR+run_target+%3D+%22sunfish-stablewifi%22+OR+run_target+%3D+%22tegu%22+OR+run_target+%3D+%22xrdk2%22%29';

      await testFilterParsing(filterString, {
        host_group: [
          'acs_beto',
          'acs:ap',
          'acs:ate-atv-experimental',
          'acs:beto',
          'acs:beto_11',
          'acs:beto_12',
          'acs:camerax',
          'acs:cash',
          'acs:gambit-agentic-tv',
          'acs:iw',
          'acs:media-stablewifi',
          'acs:pixcam-gca',
          'acs:sf',
          'atc:aap',
          'atc:abc',
          'atc:adapt',
          'atc:adb',
          'atc:assistant-mh',
          'atc:ate-atv-camera-tf-test',
          'atc:ate-dockerized-tf-auto-experimental',
          'atc:ate-main',
          'atc:ate-main-high-concurrency',
          'atc:ate-mainline',
          'atc:ate-mainline-sim',
          'atc:ate-mokey',
          'atc:ate-mokey-presubmit',
          'atc:ate-presubmit',
          'atc:ate-presubmit-high-concurrency',
          'atc:ate-tv-cec',
          'atc:ate-tv-experimental',
          'atc:ate-tv-keystone',
          'atc:ate-tv-lowram',
          'atc:ate-tv-stable',
          'atc:ate-tv-unstable',
          'atc:ate-wembley',
          'atc:ate-wembley-presubmit',
          'atc:ate-xr',
          'atc:auto',
          'atc:auto-geo',
          'atc:auto-mh',
          'atc:camera',
          'atc:cash',
          'atc:crystalball',
          'atc:crystalball-copper-cage',
          'atc:crystalball-presubmit',
          'atc:cts',
          'atc:decommission',
          'atc:dual-sim',
          'atc:haiku-mh',
          'atc:hawkeye-mh',
          'atc:media',
          'atc:mh-androidtv',
          'atc:mh-haiku',
          'atc:perf',
          'atc:pixcam-gca',
          'atc:platinum',
          'atc:platinum-wifi',
          'atc:presubmit-cw-standalone',
          'atc:rubidium-mh',
          'atc:sim',
          'atc:unbundled',
          'atc:wear-mh',
          'atc:XR-1265',
          'ate-dockerized-tf-ht-tv',
          'ate-dockerized-tf-tv',
          'ate-dockerized-tf-tv-plus',
          'ate-dockerized-tf-tv-plus-xts',
          'ate-dockerized-tf-tv-xts',
          'dockerized-tf-ate-staging',
          'dockerized-tf-nugget-os',
          'lab-unattended-recovery',
          'wear-mac',
          'wear:mh',
          '(Blank)',
        ],
        lab_name: ['acs', 'atc'],
        run_target: [
          'atoll',
          'aurora',
          'austin',
          'bramble',
          'bramble-stablewifi',
          'coral',
          'coral-stablewifi',
          'felix',
          'felix-kioxia-256gb',
          'felix-kioxia-256gb-facebook-main',
          'felix-kioxia-256gb-instagram-main',
          'felix-kioxia-256gb-snapchat-main',
          'felix-kioxia-256gb-wechat-main',
          'felix-micron-256gb',
          'felix-micron-256gb-twitter-main',
          'felix-micron-256gb-whatsapp-main',
          'husky',
          'husky-kioxia-256gb',
          'husky-samsung-128gb',
          'husky-samsung-128gb-copper-cage',
          'husky-samsung-128gb-dpm',
          'husky-samsung-128gb-instagram-main',
          'husky-samsung-128gb-power',
          'husky-samsung-128gb-power-whatsapp',
          'husky-samsung-128gb-snapchat-main',
          'husky-samsung-128gb-twitter-main',
          'husky-samsung-128gb-wechat-main',
          'husky-samsung-128gb-whatsapp-main',
          'husky-skhynix-128gb',
          'husky-skhynix-128gb-facebook-main',
          'husky-skhynix-128gb-facebook-pgagnostic',
          'husky-skhynix-128gb-pcmark-main',
          'husky-stablewifi',
          'kalama',
          'kodiak',
          'kodiak_2',
          'kodiak_2-micron-256gb',
          'kodiak_2-samsung-256gb',
          'kodiak-micron-1024gb',
          'kodiak-samsung-256gb',
          'kona',
          'menari_btwifi',
          'menari_lte',
          'meridian_btwifi',
          'meridian_lte',
          'merlinnfc',
          'milanf',
          'raven',
          'raven-camsim',
          'raven-kioxia-128gb',
          'raven-kioxia-128gb-facebook-main',
          'raven-kioxia-128gb-netflix-main',
          'raven-kioxia-128gb-twitter-main',
          'raven-skhynix-128gb',
          'raven-skhynix-128gb-power',
          'raven-skhynix-256gb',
          'raven-skhynix-256gb-pcmark-main',
          'raven-toshiba-256gb-wechat-main',
          'raven-toshiba-256gb-whatsapp-main',
          'sailfish',
          'sailfish-stablewifi',
          'stallion',
          'sunfish',
          'sunfish-stablewifi',
          'tegu',
          'xrdk2',
        ],
      });
    });

    // eslint-disable-next-line jest/expect-expect
    it('should handle full golden example from go/atc-fc-3', async () => {
      const filterString =
        // eslint-disable-next-line max-len
        '%28NOT+host_group%3A*+OR+%28host_group+%3D+%22acs_beto%22+OR+host_group+%3D+%22acs%3Aap%22+OR+host_group+%3D+%22acs%3Aate-atv-experimental%22+OR+host_group+%3D+%22acs%3Abeto%22+OR+host_group+%3D+%22acs%3Abeto_11%22+OR+host_group+%3D+%22acs%3Abeto_12%22+OR+host_group+%3D+%22acs%3Acamerax%22+OR+host_group+%3D+%22acs%3Acash%22+OR+host_group+%3D+%22acs%3Agambit-agentic-tv%22+OR+host_group+%3D+%22acs%3Aiw%22+OR+host_group+%3D+%22acs%3Amedia-stablewifi%22+OR+host_group+%3D+%22acs%3Apixcam-gca%22+OR+host_group+%3D+%22acs%3Asf%22+OR+host_group+%3D+%22atc%3Aaap%22+OR+host_group+%3D+%22atc%3Aabc%22+OR+host_group+%3D+%22atc%3Aadapt%22+OR+host_group+%3D+%22atc%3Aadb%22+OR+host_group+%3D+%22atc%3Aassistant-mh%22+OR+host_group+%3D+%22atc%3Aate-atv-camera-tf-test%22+OR+host_group+%3D+%22atc%3Aate-dockerized-tf-auto-experimental%22+OR+host_group+%3D+%22atc%3Aate-main%22+OR+host_group+%3D+%22atc%3Aate-main-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-mainline%22+OR+host_group+%3D+%22atc%3Aate-mainline-sim%22+OR+host_group+%3D+%22atc%3Aate-mokey%22+OR+host_group+%3D+%22atc%3Aate-mokey-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-tv-cec%22+OR+host_group+%3D+%22atc%3Aate-tv-experimental%22+OR+host_group+%3D+%22atc%3Aate-tv-keystone%22+OR+host_group+%3D+%22atc%3Aate-tv-lowram%22+OR+host_group+%3D+%22atc%3Aate-tv-stable%22+OR+host_group+%3D+%22atc%3Aate-tv-unstable%22+OR+host_group+%3D+%22atc%3Aate-wembley%22+OR+host_group+%3D+%22atc%3Aate-wembley-presubmit%22+OR+host_group+%3D+%22atc%3Aate-xr%22+OR+host_group+%3D+%22atc%3Aauto%22+OR+host_group+%3D+%22atc%3Aauto-geo%22+OR+host_group+%3D+%22atc%3Aauto-mh%22+OR+host_group+%3D+%22atc%3Acamera%22+OR+host_group+%3D+%22atc%3Acash%22+OR+host_group+%3D+%22atc%3Acrystalball%22+OR+host_group+%3D+%22atc%3Acrystalball-copper-cage%22+OR+host_group+%3D+%22atc%3Acrystalball-presubmit%22+OR+host_group+%3D+%22atc%3Acts%22+OR+host_group+%3D+%22atc%3Adecommission%22+OR+host_group+%3D+%22atc%3Adual-sim%22+OR+host_group+%3D+%22atc%3Ahaiku-mh%22+OR+host_group+%3D+%22atc%3Ahawkeye-mh%22+OR+host_group+%3D+%22atc%3Amedia%22+OR+host_group+%3D+%22atc%3Amh-androidtv%22+OR+host_group+%3D+%22atc%3Amh-haiku%22+OR+host_group+%3D+%22atc%3Aperf%22+OR+host_group+%3D+%22atc%3Apixcam-gca%22+OR+host_group+%3D+%22atc%3Aplatinum%22+OR+host_group+%3D+%22atc%3Aplatinum-wifi%22+OR+host_group+%3D+%22atc%3Apresubmit-cw-standalone%22+OR+host_group+%3D+%22atc%3Arubidium-mh%22+OR+host_group+%3D+%22atc%3Asim%22+OR+host_group+%3D+%22atc%3Aunbundled%22+OR+host_group+%3D+%22atc%3Awear-mh%22+OR+host_group+%3D+%22atc%3AXR-1265%22+OR+host_group+%3D+%22ate-dockerized-tf-ht-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus-xts%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-xts%22+OR+host_group+%3D+%22dockerized-tf-ate-staging%22+OR+host_group+%3D+%22dockerized-tf-nugget-os%22+OR+host_group+%3D+%22lab-unattended-recovery%22+OR+host_group+%3D+%22wear-mac%22+OR+host_group+%3D+%22wear%3Amh%22%29%29+AND+%28lab_name+%3D+%22acs%22+OR+lab_name+%3D+%22atc%22%29+AND+%28run_target+%3D+%22barbet%22+OR+run_target+%3D+%22barbet-stablewifi%22+OR+run_target+%3D+%22begonia%22+OR+run_target+%3D+%22bengal%22+OR+run_target+%3D+%22bf6eu%22+OR+run_target+%3D+%22bullhead%22+OR+run_target+%3D+%22caiman%22+OR+run_target+%3D+%22canoe%22+OR+run_target+%3D+%22grizzly%22+OR+run_target+%3D+%22grizzly_2%22+OR+run_target+%3D+%22grizzly_2-kioxia-256gb%22+OR+run_target+%3D+%22grizzly_2-samsung-256gb%22+OR+run_target+%3D+%22lahaina%22+OR+run_target+%3D+%22lga%22+OR+run_target+%3D+%22lila%22+OR+run_target+%3D+%22lito%22+OR+run_target+%3D+%22luna%22+OR+run_target+%3D+%22lynx%22+OR+run_target+%3D+%22lynx-camsim%22+OR+run_target+%3D+%22m168%22+OR+run_target+%3D+%22mokey%22+OR+run_target+%3D+%22mokey_go32%22+OR+run_target+%3D+%22msm8916%22+OR+run_target+%3D+%22msm8937%22+OR+run_target+%3D+%22msm8952%22+OR+run_target+%3D+%22msm8953%22+OR+run_target+%3D+%22msm8998%22+OR+run_target+%3D+%22msmnile%22+OR+run_target+%3D+%22mustang%22+OR+run_target+%3D+%22mustang-kioxia-256gb%22+OR+run_target+%3D+%22mustang-micron-256gb%22+OR+run_target+%3D+%22mustang-micron-256gb-facebook-main%22+OR+run_target+%3D+%22mustang-micron-512gb%22+OR+run_target+%3D+%22mustang-skhynix-529gb%22+OR+run_target+%3D+%22oriole%22+OR+run_target+%3D+%22oriole-camsim%22+OR+run_target+%3D+%22oriole-kioxia-128gb%22+OR+run_target+%3D+%22oriole-kioxia-128gb-instagram-main%22+OR+run_target+%3D+%22oriole-kioxia-128gb-snapchat-main%22+OR+run_target+%3D+%22oriole-kioxia-128gb-twitter-main%22+OR+run_target+%3D+%22oriole-kioxia-128gb-whatsapp-main%22+OR+run_target+%3D+%22oriole-skhynix-128gb%22+OR+run_target+%3D+%22oriole-skhynix-128gb-copper-cage%22+OR+run_target+%3D+%22oriole-skhynix-128gb-facebook-main%22+OR+run_target+%3D+%22oriole-skhynix-128gb-pcmarkstorage-main%22+OR+run_target+%3D+%22oriole-skhynix-256gb-wechat-main%22+OR+run_target+%3D+%22oriole-stablewifi%22+OR+run_target+%3D+%22r11%22+OR+run_target+%3D+%22r11btwifi%22+OR+run_target+%3D+%22redfin%22+OR+run_target+%3D+%22shiba%22+OR+run_target+%3D+%22shiba-camsim%22+OR+run_target+%3D+%22shiba-kioxia-256gb%22+OR+run_target+%3D+%22shiba-micron-128gb%22+OR+run_target+%3D+%22shiba-micron-128gb-instagram-main%22+OR+run_target+%3D+%22shiba-micron-128gb-pcmark-main%22+OR+run_target+%3D+%22shiba-micron-128gb-twitter-main%22+OR+run_target+%3D+%22shiba-micron-128gb-wechat-main%22+OR+run_target+%3D+%22shiba-micron-128gb-whatsapp-main%22+OR+run_target+%3D+%22shiba-samsung-128gb%22+OR+run_target+%3D+%22shiba-samsung-128gb-facebook-main%22+OR+run_target+%3D+%22shiba-stablewifi%22+OR+run_target+%3D+%22taro%22+OR+run_target+%3D+%22tonga%22+OR+run_target+%3D+%22trinket%22+OR+run_target+%3D+%22vayu%22+OR+run_target+%3D+%22vienna%22+OR+run_target+%3D+%22vili%22%29';

      await testFilterParsing(filterString, {
        host_group: [
          'acs_beto',
          'acs:ap',
          'acs:ate-atv-experimental',
          'acs:beto',
          'acs:beto_11',
          'acs:beto_12',
          'acs:camerax',
          'acs:cash',
          'acs:gambit-agentic-tv',
          'acs:iw',
          'acs:media-stablewifi',
          'acs:pixcam-gca',
          'acs:sf',
          'atc:aap',
          'atc:abc',
          'atc:adapt',
          'atc:adb',
          'atc:assistant-mh',
          'atc:ate-atv-camera-tf-test',
          'atc:ate-dockerized-tf-auto-experimental',
          'atc:ate-main',
          'atc:ate-main-high-concurrency',
          'atc:ate-mainline',
          'atc:ate-mainline-sim',
          'atc:ate-mokey',
          'atc:ate-mokey-presubmit',
          'atc:ate-presubmit',
          'atc:ate-presubmit-high-concurrency',
          'atc:ate-tv-cec',
          'atc:ate-tv-experimental',
          'atc:ate-tv-keystone',
          'atc:ate-tv-lowram',
          'atc:ate-tv-stable',
          'atc:ate-tv-unstable',
          'atc:ate-wembley',
          'atc:ate-wembley-presubmit',
          'atc:ate-xr',
          'atc:auto',
          'atc:auto-geo',
          'atc:auto-mh',
          'atc:camera',
          'atc:cash',
          'atc:crystalball',
          'atc:crystalball-copper-cage',
          'atc:crystalball-presubmit',
          'atc:cts',
          'atc:decommission',
          'atc:dual-sim',
          'atc:haiku-mh',
          'atc:hawkeye-mh',
          'atc:media',
          'atc:mh-androidtv',
          'atc:mh-haiku',
          'atc:perf',
          'atc:pixcam-gca',
          'atc:platinum',
          'atc:platinum-wifi',
          'atc:presubmit-cw-standalone',
          'atc:rubidium-mh',
          'atc:sim',
          'atc:unbundled',
          'atc:wear-mh',
          'atc:XR-1265',
          'ate-dockerized-tf-ht-tv',
          'ate-dockerized-tf-tv',
          'ate-dockerized-tf-tv-plus',
          'ate-dockerized-tf-tv-plus-xts',
          'ate-dockerized-tf-tv-xts',
          'dockerized-tf-ate-staging',
          'dockerized-tf-nugget-os',
          'lab-unattended-recovery',
          'wear-mac',
          'wear:mh',
          '(Blank)',
        ],
        lab_name: ['acs', 'atc'],
        run_target: [
          'barbet',
          'barbet-stablewifi',
          'begonia',
          'bengal',
          'bf6eu',
          'bullhead',
          'caiman',
          'canoe',
          'grizzly',
          'grizzly_2',
          'grizzly_2-kioxia-256gb',
          'grizzly_2-samsung-256gb',
          'lahaina',
          'lga',
          'lila',
          'lito',
          'luna',
          'lynx',
          'lynx-camsim',
          'm168',
          'mokey',
          'mokey_go32',
          'msm8916',
          'msm8937',
          'msm8952',
          'msm8953',
          'msm8998',
          'msmnile',
          'mustang',
          'mustang-kioxia-256gb',
          'mustang-micron-256gb',
          'mustang-micron-256gb-facebook-main',
          'mustang-micron-512gb',
          'mustang-skhynix-529gb',
          'oriole',
          'oriole-camsim',
          'oriole-kioxia-128gb',
          'oriole-kioxia-128gb-instagram-main',
          'oriole-kioxia-128gb-snapchat-main',
          'oriole-kioxia-128gb-twitter-main',
          'oriole-kioxia-128gb-whatsapp-main',
          'oriole-skhynix-128gb',
          'oriole-skhynix-128gb-copper-cage',
          'oriole-skhynix-128gb-facebook-main',
          'oriole-skhynix-128gb-pcmarkstorage-main',
          'oriole-skhynix-256gb-wechat-main',
          'oriole-stablewifi',
          'r11',
          'r11btwifi',
          'redfin',
          'shiba',
          'shiba-camsim',
          'shiba-kioxia-256gb',
          'shiba-micron-128gb',
          'shiba-micron-128gb-instagram-main',
          'shiba-micron-128gb-pcmark-main',
          'shiba-micron-128gb-twitter-main',
          'shiba-micron-128gb-wechat-main',
          'shiba-micron-128gb-whatsapp-main',
          'shiba-samsung-128gb',
          'shiba-samsung-128gb-facebook-main',
          'shiba-stablewifi',
          'taro',
          'tonga',
          'trinket',
          'vayu',
          'vienna',
          'vili',
        ],
      });
    });

    // eslint-disable-next-line jest/expect-expect
    it('should handle full golden example from go/atc-fc-4', async () => {
      const filterString =
        // eslint-disable-next-line max-len
        '%28NOT+host_group%3A*+OR+%28host_group+%3D+%22acs_beto%22+OR+host_group+%3D+%22acs%3Aap%22+OR+host_group+%3D+%22acs%3Aate-atv-experimental%22+OR+host_group+%3D+%22acs%3Abeto%22+OR+host_group+%3D+%22acs%3Abeto_11%22+OR+host_group+%3D+%22acs%3Abeto_12%22+OR+host_group+%3D+%22acs%3Acamerax%22+OR+host_group+%3D+%22acs%3Acash%22+OR+host_group+%3D+%22acs%3Agambit-agentic-tv%22+OR+host_group+%3D+%22acs%3Aiw%22+OR+host_group+%3D+%22acs%3Amedia-stablewifi%22+OR+host_group+%3D+%22acs%3Apixcam-gca%22+OR+host_group+%3D+%22acs%3Asf%22+OR+host_group+%3D+%22atc%3Aaap%22+OR+host_group+%3D+%22atc%3Aabc%22+OR+host_group+%3D+%22atc%3Aadapt%22+OR+host_group+%3D+%22atc%3Aadb%22+OR+host_group+%3D+%22atc%3Aassistant-mh%22+OR+host_group+%3D+%22atc%3Aate-atv-camera-tf-test%22+OR+host_group+%3D+%22atc%3Aate-dockerized-tf-auto-experimental%22+OR+host_group+%3D+%22atc%3Aate-main%22+OR+host_group+%3D+%22atc%3Aate-main-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-mainline%22+OR+host_group+%3D+%22atc%3Aate-mainline-sim%22+OR+host_group+%3D+%22atc%3Aate-mokey%22+OR+host_group+%3D+%22atc%3Aate-mokey-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit%22+OR+host_group+%3D+%22atc%3Aate-presubmit-high-concurrency%22+OR+host_group+%3D+%22atc%3Aate-tv-cec%22+OR+host_group+%3D+%22atc%3Aate-tv-experimental%22+OR+host_group+%3D+%22atc%3Aate-tv-keystone%22+OR+host_group+%3D+%22atc%3Aate-tv-lowram%22+OR+host_group+%3D+%22atc%3Aate-tv-stable%22+OR+host_group+%3D+%22atc%3Aate-tv-unstable%22+OR+host_group+%3D+%22atc%3Aate-wembley%22+OR+host_group+%3D+%22atc%3Aate-wembley-presubmit%22+OR+host_group+%3D+%22atc%3Aate-xr%22+OR+host_group+%3D+%22atc%3Aauto%22+OR+host_group+%3D+%22atc%3Aauto-geo%22+OR+host_group+%3D+%22atc%3Aauto-mh%22+OR+host_group+%3D+%22atc%3Acamera%22+OR+host_group+%3D+%22atc%3Acash%22+OR+host_group+%3D+%22atc%3Acrystalball%22+OR+host_group+%3D+%22atc%3Acrystalball-copper-cage%22+OR+host_group+%3D+%22atc%3Acrystalball-presubmit%22+OR+host_group+%3D+%22atc%3Acts%22+OR+host_group+%3D+%22atc%3Adecommission%22+OR+host_group+%3D+%22atc%3Adual-sim%22+OR+host_group+%3D+%22atc%3Ahaiku-mh%22+OR+host_group+%3D+%22atc%3Ahawkeye-mh%22+OR+host_group+%3D+%22atc%3Amedia%22+OR+host_group+%3D+%22atc%3Amh-androidtv%22+OR+host_group+%3D+%22atc%3Amh-haiku%22+OR+host_group+%3D+%22atc%3Aperf%22+OR+host_group+%3D+%22atc%3Apixcam-gca%22+OR+host_group+%3D+%22atc%3Aplatinum%22+OR+host_group+%3D+%22atc%3Aplatinum-wifi%22+OR+host_group+%3D+%22atc%3Apresubmit-cw-standalone%22+OR+host_group+%3D+%22atc%3Arubidium-mh%22+OR+host_group+%3D+%22atc%3Asim%22+OR+host_group+%3D+%22atc%3Aunbundled%22+OR+host_group+%3D+%22atc%3Awear-mh%22+OR+host_group+%3D+%22atc%3AXR-1265%22+OR+host_group+%3D+%22ate-dockerized-tf-ht-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-plus-xts%22+OR+host_group+%3D+%22ate-dockerized-tf-tv-xts%22+OR+host_group+%3D+%22dockerized-tf-ate-staging%22+OR+host_group+%3D+%22dockerized-tf-nugget-os%22+OR+host_group+%3D+%22lab-unattended-recovery%22+OR+host_group+%3D+%22wear-mac%22+OR+host_group+%3D+%22wear%3Amh%22%29%29+AND+%28lab_name+%3D+%22acs%22+OR+lab_name+%3D+%22atc%22%29+AND+%28run_target+%3D+%22blazer%22+OR+run_target+%3D+%22blazer-micron-256g%22+OR+run_target+%3D+%22blazer-micron-256gb%22+OR+run_target+%3D+%22blazer-micron-512gb%22+OR+run_target+%3D+%22blazer-samsung-128gb%22+OR+run_target+%3D+%22blazer-skhynix-1064gb%22+OR+run_target+%3D+%22comet%22+OR+run_target+%3D+%22comet-kioxia-256gb%22+OR+run_target+%3D+%22comet-kioxia-512gb%22+OR+run_target+%3D+%22comet-kioxia-512gb-facebook-main%22+OR+run_target+%3D+%22comet-micron-512gb-instagram-main%22+OR+run_target+%3D+%22comet-micron-512gb-snapchat-main%22+OR+run_target+%3D+%22comet-micron-512gb-wechat-main%22+OR+run_target+%3D+%22deb%22+OR+run_target+%3D+%22diting%22+OR+run_target+%3D+%22dragon%22+OR+run_target+%3D+%22dub-l21%22+OR+run_target+%3D+%22exynos2100%22+OR+run_target+%3D+%22exynos3830%22+OR+run_target+%3D+%22exynos7870%22+OR+run_target+%3D+%22exynos7884b%22+OR+run_target+%3D+%22exynos7904%22+OR+run_target+%3D+%22exynos850%22+OR+run_target+%3D+%22exynos9610%22+OR+run_target+%3D+%22exynos9611%22+OR+run_target+%3D+%22exynos980%22+OR+run_target+%3D+%22exynos9810%22+OR+run_target+%3D+%22exynos9820%22+OR+run_target+%3D+%22flame%22+OR+run_target+%3D+%22flame-stablewifi%22+OR+run_target+%3D+%22oppo6762_18540%22+OR+run_target+%3D+%22oppo6765%22+OR+run_target+%3D+%22oppo6765_19451%22+OR+run_target+%3D+%22oppo6765_19581%22+OR+run_target+%3D+%22oppo6771%22+OR+run_target+%3D+%22p11%22+OR+run_target+%3D+%22panther%22+OR+run_target+%3D+%22panther-camsim%22+OR+run_target+%3D+%22panther-micron-128gb%22+OR+run_target+%3D+%22panther-micron-128gb-facebook-main%22+OR+run_target+%3D+%22panther-micron-128gb-snapchat-main%22+OR+run_target+%3D+%22panther-skhynix-128gb%22+OR+run_target+%3D+%22panther-skhynix-128gb-dpm%22+OR+run_target+%3D+%22panther-skhynix-128gb-twitter-main%22+OR+run_target+%3D+%22panther-skhynix-128gb-wechat-main%22+OR+run_target+%3D+%22panther-skhynix-128gb-whatsapp-main%22+OR+run_target+%3D+%22panther-stablewifi%22+OR+run_target+%3D+%22qc_reference_phone%22+OR+run_target+%3D+%22qc_reference_phone%3Ayt-x705f%22+OR+run_target+%3D+%22rango%22+OR+run_target+%3D+%22rango-kioxia-256gb%22+OR+run_target+%3D+%22rm6762_18540%22+OR+run_target+%3D+%22rm6769%22+OR+run_target+%3D+%22rmx3231%22+OR+run_target+%3D+%22rmx3263%22+OR+run_target+%3D+%22rmx3581%22+OR+run_target+%3D+%22s5e5515%22+OR+run_target+%3D+%22s5e5535%22+OR+run_target+%3D+%22s5e8535%22+OR+run_target+%3D+%22s5e8825%22+OR+run_target+%3D+%22s5e8835%22+OR+run_target+%3D+%22s5e8845%22+OR+run_target+%3D+%22s5e9925%22+OR+run_target+%3D+%22s5e9945%22+OR+run_target+%3D+%22s5e9955%22+OR+run_target+%3D+%22s96116ra1%22+OR+run_target+%3D+%22seahawk%22+OR+run_target+%3D+%22selene%22+OR+run_target+%3D+%22seluna%22+OR+run_target+%3D+%22serenity%22+OR+run_target+%3D+%22taimen%22+OR+run_target+%3D+%22taimen-stablewifi%22+OR+run_target+%3D+%22tangor%22+OR+run_target+%3D+%22tangorpro%22+OR+run_target+%3D+%22tangorpro-kioxia-256gb%22+OR+run_target+%3D+%22tangorpro-micron-128gb%22+OR+run_target+%3D+%22tangorpro-micron-256gb%22+OR+run_target+%3D+%22tangorpro-skhynix-128gb%22+OR+run_target+%3D+%22tokay%22+OR+run_target+%3D+%22tokay-camsim%22+OR+run_target+%3D+%22tokay-micron-256gb-wechat-main%22+OR+run_target+%3D+%22tokay-samsung-128gb%22+OR+run_target+%3D+%22tokay-samsung-128gb-snapchat-main%22+OR+run_target+%3D+%22tokay-samsung-256gb%22+OR+run_target+%3D+%22tokay-samsung-256gb-facebook-main%22+OR+run_target+%3D+%22tokay-samsung-256gb-instagram-main%22+OR+run_target+%3D+%22tokay-skhynix-128gb%22+OR+run_target+%3D+%22tokay-skhynix-128gb-pcmark-main%22+OR+run_target+%3D+%22wembley%22+OR+run_target+%3D+%22yogi%22+OR+run_target+%3D+%22yogi-micron-256gb%22+OR+run_target+%3D+%22yogi-samsung-256gb%22%29';

      await testFilterParsing(filterString, {
        host_group: [
          'acs_beto',
          'acs:ap',
          'acs:ate-atv-experimental',
          'acs:beto',
          'acs:beto_11',
          'acs:beto_12',
          'acs:camerax',
          'acs:cash',
          'acs:gambit-agentic-tv',
          'acs:iw',
          'acs:media-stablewifi',
          'acs:pixcam-gca',
          'acs:sf',
          'atc:aap',
          'atc:abc',
          'atc:adapt',
          'atc:adb',
          'atc:assistant-mh',
          'atc:ate-atv-camera-tf-test',
          'atc:ate-dockerized-tf-auto-experimental',
          'atc:ate-main',
          'atc:ate-main-high-concurrency',
          'atc:ate-mainline',
          'atc:ate-mainline-sim',
          'atc:ate-mokey',
          'atc:ate-mokey-presubmit',
          'atc:ate-presubmit',
          'atc:ate-presubmit-high-concurrency',
          'atc:ate-tv-cec',
          'atc:ate-tv-experimental',
          'atc:ate-tv-keystone',
          'atc:ate-tv-lowram',
          'atc:ate-tv-stable',
          'atc:ate-tv-unstable',
          'atc:ate-wembley',
          'atc:ate-wembley-presubmit',
          'atc:ate-xr',
          'atc:auto',
          'atc:auto-geo',
          'atc:auto-mh',
          'atc:camera',
          'atc:cash',
          'atc:crystalball',
          'atc:crystalball-copper-cage',
          'atc:crystalball-presubmit',
          'atc:cts',
          'atc:decommission',
          'atc:dual-sim',
          'atc:haiku-mh',
          'atc:hawkeye-mh',
          'atc:media',
          'atc:mh-androidtv',
          'atc:mh-haiku',
          'atc:perf',
          'atc:pixcam-gca',
          'atc:platinum',
          'atc:platinum-wifi',
          'atc:presubmit-cw-standalone',
          'atc:rubidium-mh',
          'atc:sim',
          'atc:unbundled',
          'atc:wear-mh',
          'atc:XR-1265',
          'ate-dockerized-tf-ht-tv',
          'ate-dockerized-tf-tv',
          'ate-dockerized-tf-tv-plus',
          'ate-dockerized-tf-tv-plus-xts',
          'ate-dockerized-tf-tv-xts',
          'dockerized-tf-ate-staging',
          'dockerized-tf-nugget-os',
          'lab-unattended-recovery',
          'wear-mac',
          'wear:mh',
          '(Blank)',
        ],
        lab_name: ['acs', 'atc'],
        run_target: [
          'blazer',
          'blazer-micron-256g',
          'blazer-micron-256gb',
          'blazer-micron-512gb',
          'blazer-samsung-128gb',
          'blazer-skhynix-1064gb',
          'comet',
          'comet-kioxia-256gb',
          'comet-kioxia-512gb',
          'comet-kioxia-512gb-facebook-main',
          'comet-micron-512gb-instagram-main',
          'comet-micron-512gb-snapchat-main',
          'comet-micron-512gb-wechat-main',
          'deb',
          'diting',
          'dragon',
          'dub-l21',
          'exynos2100',
          'exynos3830',
          'exynos7870',
          'exynos7884b',
          'exynos7904',
          'exynos850',
          'exynos9610',
          'exynos9611',
          'exynos980',
          'exynos9810',
          'exynos9820',
          'flame',
          'flame-stablewifi',
          'oppo6762_18540',
          'oppo6765',
          'oppo6765_19451',
          'oppo6765_19581',
          'oppo6771',
          'p11',
          'panther',
          'panther-camsim',
          'panther-micron-128gb',
          'panther-micron-128gb-facebook-main',
          'panther-micron-128gb-snapchat-main',
          'panther-skhynix-128gb',
          'panther-skhynix-128gb-dpm',
          'panther-skhynix-128gb-twitter-main',
          'panther-skhynix-128gb-wechat-main',
          'panther-skhynix-128gb-whatsapp-main',
          'panther-stablewifi',
          'qc_reference_phone',
          'qc_reference_phone:yt-x705f',
          'rango',
          'rango-kioxia-256gb',
          'rm6762_18540',
          'rm6769',
          'rmx3231',
          'rmx3263',
          'rmx3581',
          's5e5515',
          's5e5535',
          's5e8535',
          's5e8825',
          's5e8835',
          's5e8845',
          's5e9925',
          's5e9945',
          's5e9955',
          's96116ra1',
          'seahawk',
          'selene',
          'seluna',
          'serenity',
          'taimen',
          'taimen-stablewifi',
          'tangor',
          'tangorpro',
          'tangorpro-kioxia-256gb',
          'tangorpro-micron-128gb',
          'tangorpro-micron-256gb',
          'tangorpro-skhynix-128gb',
          'tokay',
          'tokay-camsim',
          'tokay-micron-256gb-wechat-main',
          'tokay-samsung-128gb',
          'tokay-samsung-128gb-snapchat-main',
          'tokay-samsung-256gb',
          'tokay-samsung-256gb-facebook-main',
          'tokay-samsung-256gb-instagram-main',
          'tokay-skhynix-128gb',
          'tokay-skhynix-128gb-pcmark-main',
          'wembley',
          'yogi',
          'yogi-micron-256gb',
          'yogi-samsung-256gb',
        ],
      });
    });
  });
});
