import { ParamKeyValuePair, useSearchParams } from 'react-router-dom';

export function useSelectedVariantGroupsParam(validVariantGroups: string[]): [string[], (selectedVariantGroups: string[], replace?: boolean) => void] {
  const [searchParams, setSearchParams] = useSearchParams();
  const groupByParam = searchParams.get('groupby') || '';
  const selectedVariantGroups: string[] = [];
  if (groupByParam) {
    const groups = groupByParam.split(',');
    for (let i = 0; i < groups.length; i++) {
      if (validVariantGroups.indexOf(groups[i]) != -1) {
        selectedVariantGroups.push(groups[i]);
      }
    }
  }

  function updateSelectedVariantGroupsParam(selectedVariantGroups: string[], replace = false) {
    const params: ParamKeyValuePair[] = [];

    for (const [k, v] of searchParams.entries()) {
      if (k !== 'groupby') {
        params.push([k, v]);
      }
    }

    params.push(['groupby', selectedVariantGroups.join(',')]);

    setSearchParams(params, {
      replace,
    });
  }

  return [selectedVariantGroups, updateSelectedVariantGroupsParam];
}
