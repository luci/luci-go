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


// Enum for kinds of principals.
const PrincipalKind = Object.freeze({
  'PRINCIPAL_KIND_UNSPECIFIED': 0,
  'IDENTITY': 1,
  'GROUP': 2,
  'GLOB': 3,
});


////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// Wrapper around the API call to look up a principal.
const doLookup = (principal) => {
  // Normalize the principal to the form the API expects. As everywhere in the
  // UI, we assume 'user:' prefix is implied in emails and globs. In addition,
  // emails all have '@' symbol and (unlike external groups such as google/a@b)
  // don't have '/', and globs all have '*' symbol. Everything else is assumed
  // to be a group.
  let isEmail = principal.indexOf('@') != -1 && principal.indexOf('/') == -1;
  let isGlob = principal.indexOf('*') != -1;
  if ((isEmail || isGlob) && principal.indexOf(':') == -1) {
    principal = 'user:' + principal;
  }

  let kind = PrincipalKind.GROUP;
  if (isGlob) {
    kind = PrincipalKind.GLOB;
  } else if (isEmail) {
    kind = PrincipalKind.IDENTITY;
  }
  const lookupPrincipalRequest = {
    'kind': kind,
    'name': principal,
  }
  return api.groupLookup(lookupPrincipalRequest);
}

// Takes subgraph returned by API and produces a summary that can be easily
// interpreted.
const interpretLookupResults = (subgraph) => {
  // Note: the principal is always represented by nodes[0] per API guarantee.
  const nodes = subgraph.nodes;
  const principalNode = nodes[0]
  const principal = principalNode.principal;


  let includers = new Map();
  const getIncluder = (groupName) => {
    if (!includers.has(groupName)) {
      includers.set(groupName, {
        'name': groupName,
        'href': common.getLookupURL(groupName),
        'includesDirectly': false,
        'includesViaGlobs': [],
        'includesIndirectly': [],
      });
    }
    return includers.get(groupName);
  }

  const enumeratePaths = (current, visitCallback) => {
    visitCallback(current);
    const lastNode = current[current.length - 1];
    if (lastNode.includedBy) {
      lastNode.includedBy.forEach(idx => {
        const node = nodes[idx];
        console.assert(current.indexOf(node) == -1);  // no cycles!
        current.push(node);
        enumeratePaths(current, visitCallback);
        current.pop();
      });
    }
  }

  const visitor = (path) => {
    console.assert(path.length > 0);
    console.assert(path[0] === principalNode);
    if (path.length == 1) {
      return;  // the trivial [principal] path
    }

    const lastNode = path[path.length - 1];
    if (lastNode.principal.kind != 'GROUP') {
      return;  // we are only interested in examining groups; skip GLOBs
    }

    let groupIncluder = getIncluder(lastNode.principal.name);
    if (path.length == 2) {
      // The entire path is 'principalNode -> lastNode', meaning group
      // 'last' includes the principal directly.
      groupIncluder.includesDirectly = true;
    } else if (path.length == 3 && path[1].principal.kind == 'GLOB') {
      // The entire path is 'principalNode -> GLOB -> lastNode', meaning
      // 'last' includes the principal via the GLOB.
      groupIncluder.includesViaGlobs.push(
        common.stripPrefix('user', path[1].principal.name)
      );
    } else {
      // Some arbitrarily long indirect inclusion path. Just record all
      // group names in it (skipping GLOBs). Skip the root principal
      // itself (path[0]) and the currrently analyzed node (path[-1]);
      // it's not useful information as it's the same for all paths.
      let groupNames = [];
      for (let i = 1; i < path.length - 1; i++) {
        if (path[i].principal.kind == 'GROUP') {
          groupNames.push(path[i].principal.name);
        }
      }
      groupIncluder.includesIndirectly.push(groupNames);
    }
  }

  enumeratePaths([principalNode], visitor);

  // Finally, massage the findings for easier display. Note that
  // directIncluders and indirectIncluders are NOT disjoint sets.
  let directIncluders = [];
  let indirectIncluders = [];
  includers.forEach(inc => {
    if (inc.includesDirectly || inc.includesViaGlobs.length > 0) {
      directIncluders.push(inc);
    }

    if (inc.includesIndirectly.length > 0) {
      // Long inclusion paths look like data dumps in UI and don't fit
      // most of the time. The most interesting components are at the
      // ends, so keep only them.
      inc.includesIndirectly = shortenInclusionPaths(inc.includesIndirectly);
      indirectIncluders.push(inc);
    }
  });

  directIncluders = common.sortGroupsByName(directIncluders);
  indirectIncluders = common.sortGroupsByName(indirectIncluders);

  // If looking up a group, there should be a link to the main group page.
  let groupHref = '';
  if (principal.kind == 'GROUP') {
    groupHref = common.getGroupPageURL(principal.name);
  }

  return {
    'principalName': common.stripPrefix('user', principal.name),
    'principalIsGroup': principal.kind == 'GROUP',
    'groupHref': groupHref,
    'includers': includers,  // will be used to construct popovers
    'directIncluders': directIncluders,
    'indirectIncluders': indirectIncluders,
  };
};

// For each long path in the list, kick out the middle and replace it with ''.
const shortenInclusionPaths = (paths) => {
  let out = [];
  let seen = new Set();

  paths.forEach(path => {
    if (path.length <= 3) {
      out.push(path); // short enough already
      return;
    }
    const shorter = [path[0], '', path[path.length - 1]];
    const key = shorter.join('\n');
    if (!seen.has(key)) {
      seen.add(key);
      out.push(shorter);
    }
  });

  return out;
};


////////////////////////////////////////////////////////////////////////////////
// Search bar (text field and button).
class SearchBar {
  constructor(element) {
    // Root DOM element.
    this.element = document.querySelector(element);

    // Text input and search button.
    this.input = this.element.querySelector('input');
    this.btn = this.element.querySelector('button');
  }

  #setDisabled(disable) {
    this.input.disabled = disable;
    this.btn.disabled = disable;
  }

  enableInteraction() {
    this.#setDisabled(false);
  }

  disableInteraction() {
    this.#setDisabled(true);
  }
}


////////////////////////////////////////////////////////////////////////////////
// Base class for a hidable component.
class HidableElement {
  constructor(element, show) {
    // Root DOM element.
    this.element = document.querySelector(element);

    // Set the initial visibility.
    if (show) {
      this.show();
    } else {
      this.hide();
    }
  }

  show() {
    this.element.style.display = 'block';
  }

  hide() {
    this.element.style.display = 'none';
  }
}


////////////////////////////////////////////////////////////////////////////////
// Component with centered spinner to represent loading.
class LoadingBox extends HidableElement {
  constructor(element) {
    super(element, false);
  }

  setLoadStatus(isLoading) {
    if (isLoading) {
      this.show();
    } else {
      this.hide();
    }
  }
}


////////////////////////////////////////////////////////////////////////////////
// Component to display all search results.
class SearchResults extends HidableElement {
  constructor(element) {
    super(element, false);
  }

  clearResults() {
    // Empty the DOM element.
    this.element.innerHTML = '';
  }

  setLookupResults(subgraph) {
    this.clearResults();

    const summary = interpretLookupResults(subgraph);

    const template = document.querySelector('#all-results-template');
    // Clone and grab elements to modify.
    const clone = template.content.cloneNode(true);
    const header = clone.querySelector('#principal-header');
    const directSection = clone.querySelector('#direct-groups');
    const indirectSection = clone.querySelector('#indirect-groups');

    // Set the principal header (i.e. what was searched).
    header.textContent = summary.principalName;
    if (summary.principalIsGroup) {
      header.setAttribute('href', summary.groupHref);
    }

    // Set the direct group inclusions.
    if (summary.directIncluders.length > 0) {
      // Empty the section to remove the default "None" result.
      directSection.innerHTML = '';

      summary.directIncluders.forEach(inc => {
        const d = new DirectResultItem(inc);
        directSection.appendChild(d.element);
      });
    }

    // Set the indirect group inclusions.
    if (summary.indirectIncluders.length > 0) {
      // Empty the section to remove the default "None" result.
      indirectSection.innerHTML = '';

      summary.indirectIncluders.forEach(inc => {
        const d = new IndirectResultItem(inc);
        indirectSection.appendChild(d.element);
      });
    }

    // Finally, add the populated template to the root DOM element.
    this.element.appendChild(clone);
  }
}


////////////////////////////////////////////////////////////////////////////////
// Base class for a singular search result.
class ResultItem {
  constructor(template_id, includer) {
    const template = document.querySelector(template_id);

    // Clone and grab elements to modify.
    const clone = template.content.cloneNode(true);
    const rootEl = clone.querySelector('div');
    const linkEl = clone.querySelector('a');

    // Set the link text and target.
    linkEl.textContent = includer.name;
    linkEl.setAttribute('href', includer.href);

    this.element = rootEl;
  }
}


////////////////////////////////////////////////////////////////////////////////
// Singular search result, representing a group the principal is directly in.
class DirectResultItem extends ResultItem {
  constructor(includer) {
    super('#direct-result-template', includer);

    // Add a description if included via GLOB.
    if (includer.includesViaGlobs.length > 0) {
      const desc = document.createElement('small');
      desc.textContent = 'via ' + includer.includesViaGlobs.join(', ');
      this.element.appendChild(desc);
    }
  }
}


////////////////////////////////////////////////////////////////////////////////
// Singular search result, representing a group the principal is indirectly in.
class IndirectResultItem extends ResultItem {
  constructor(includer) {
    super('#indirect-result-template', includer);

    // TODO: set tooltip details for inclusion path.
  }
}


////////////////////////////////////////////////////////////////////////////////


window.onload = () => {
  const searchBar = new SearchBar('#search-bar');
  const loadingBox = new LoadingBox('#loading-box');
  const searchResults = new SearchResults('#all-results');

  // TODO: record principal searched in the URL, and check it on load.

  const doSearch = () => {
    const userInput = searchBar.input.value;
    if (!userInput) {
      return;
    }

    searchBar.disableInteraction();
    searchResults.hide();
    loadingBox.setLoadStatus(true);

    doLookup(userInput)
      .then((response) => {
        searchResults.setLookupResults(response);
        searchResults.show();
      })
      .catch((err) => {
        // TODO: replace this with an error alert.
        console.error('Lookup failed:', err);
      })
      .finally(() => {
        loadingBox.setLoadStatus(false);
        searchBar.enableInteraction();
      });
  }

  // Search if the search button is clicked.
  searchBar.btn.addEventListener('click', (e) => {
    doSearch();
  });

  // Search if "Enter" is hit.
  searchBar.input.addEventListener('keyup', (e) => {
    if (e.keyCode == 13) {
      doSearch();
    }
  });

  searchBar.enableInteraction();
}
