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


const INTEGRATED_UI_LOOKUP_ROOT = "/ui/auth/lookup";

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

  return {
    'principalName': principal.name,
    'principalKind': principal.kind,
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
// Component to display all search results.
class SearchResults extends common.HidableElement {
  constructor(element) {
    super(element, false);

    // Popovers for details of indirect results.
    this.popovers = [];
    // Tooltip for change link.
    this.tooltip = null;
  }

  clearResults() {
    // Dispose of popovers that were for previous results, so there aren't any
    // orphaned elements floating about.
    this.popovers.forEach((p) => {
      p.dispose();
    })
    this.popovers = [];

    // Dispose of tooltip for link to changes.
    if (this.tooltip) {
      this.tooltip.dispose();
    }
    this.tooltip = null;

    // Empty the DOM element.
    this.element.innerHTML = '';
  }

  setLookupResults(subgraph) {
    this.clearResults();

    const summary = interpretLookupResults(subgraph);

    const template = document.querySelector('#all-results-template');
    // Clone and grab elements to modify.
    const clone = template.content.cloneNode(true);
    const headerLink = clone.querySelector('#principal-link');
    const directSection = clone.querySelector('#direct-groups');
    const indirectSection = clone.querySelector('#indirect-groups');

    // Set the principal value (i.e. what was searched).
    const principalValue = common.stripPrefix('user', summary.principalName);
    headerLink.textContent = principalValue;

    // Set links.
    let changeLinkTarget = null;
    if (summary.principalKind == 'GROUP') {
      // There should be a link to the main group page.
      headerLink.setAttribute('href', common.getGroupPageURL(summary.principalName));
      // There should be a link to changes of the group.
      changeLinkTarget = common.getChangeLogTargetURL('AuthGroup',principalValue);
    } else if (summary.principalKind == 'IDENTITY') {
      // There should be a link to changes made by the identity.
      changeLinkTarget = common.getChangeLogModifierURL(principalValue);
    }
    if (changeLinkTarget) {
      // Create an icon that links to changes (either the group history, or
      // changes to groups made by the identity).
      const iconTemplate = document.querySelector('#change-log-icon-template');
      const iconClone = iconTemplate.content.cloneNode(true);
      const changeLink = iconClone.querySelector('#change-log-link');
      changeLink.setAttribute('href', changeLinkTarget);

      const header = clone.querySelector('#principal-header');
      header.appendChild(changeLink);

      // Enable tooltip for the change log link.
      this.tooltip = new bootstrap.Tooltip(changeLink);
    }

    // Set the direct group inclusions.
    if (summary.directIncluders.length > 0) {
      // Empty the section to remove the default "None" result.
      directSection.innerHTML = '';

      summary.directIncluders.forEach(inc => {
        const result = new DirectResultItem(inc);
        directSection.appendChild(result.element);
      });
    }

    // Set the indirect group inclusions.
    if (summary.indirectIncluders.length > 0) {
      // Empty the section to remove the default "None" result.
      indirectSection.innerHTML = '';

      summary.indirectIncluders.forEach(inc => {
        const result = new IndirectResultItem(inc);
        indirectSection.appendChild(result.element);
        // Keep track of the popover so it can be disposed of later.
        this.popovers.push(result.popover);
      });
    }

    // Finally, add the populated template to the root DOM element.
    this.element.appendChild(clone);
  }
}


////////////////////////////////////////////////////////////////////////////////
// Base class for a singular search result.
class ResultItem {
  constructor(includer) {
    const template = document.querySelector('#result-template');

    // Clone and grab elements to modify.
    const clone = template.content.cloneNode(true);
    this.element = clone.querySelector('div');
    this.link = clone.querySelector('a');

    // Set the link text and target.
    this.link.textContent = includer.name;
    this.link.setAttribute('href', includer.href);
  }
}


////////////////////////////////////////////////////////////////////////////////
// Singular search result, representing a group the principal is directly in.
class DirectResultItem extends ResultItem {
  constructor(includer) {
    super(includer);

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
    super(includer);

    // Construct an element with the indirect inclusion details.
    const popoverContent = document.createElement('div');
    includer.includesIndirectly.forEach((groupNames) => {
      // Replace empty strings with ellipses.
      const displayNames = groupNames.map(g => g === '' ? '\u2026' : g);

      const pathResult = document.createElement('div');
      pathResult.classList.add('small', 'my-2');
      pathResult.textContent = displayNames.join(' \u2192 ');
      popoverContent.appendChild(pathResult);
    });

    // Create a popover with the details content.
    this.popover = new bootstrap.Popover(this.link, {
      container: 'body',
      content: popoverContent,
      html: true,
      placement: 'left',
      title: 'Included via',
      trigger: 'hover',
    });
  }
}


////////////////////////////////////////////////////////////////////////////////
// Address bar manipulation.

const getCurrentPrincipalInURL = () => {
  return common.getQueryParameter('p');
}

const setCurrentPrincipalInURL = (principal) => {
  if (getCurrentPrincipalInURL() != principal) {
    window.history.pushState({ 'principal': principal }, null,
      common.getLookupURL(principal));
  }
}

const onCurrentPrincipalInURLChange = (cb) => {
  window.onpopstate = function (event) {
    var s = event.state;
    if (s && s.hasOwnProperty('principal')) {
      cb(s.principal);
    }
  };
}


////////////////////////////////////////////////////////////////////////////////

const isGroup = (principal) => {
  if (!principal) {
    return false;
  }
  if (principal.indexOf('*') != -1) {
    // Must be glob.
    return false;
  }
  if (principal.indexOf('@') != -1 && principal.indexOf('/') == -1) {
    // Must be email.
    return false
  }

  return true;
}

window.onload = () => {
  const integratedUIAlert = new common.IntegratedUIAlert('#integrated-ui-alert-container');
  integratedUIAlert.setLink(INTEGRATED_UI_LOOKUP_ROOT);
  const searchBar = new SearchBar('#search-bar');
  const loadingBox = new common.LoadingBox('#loading-box-placeholder');
  const searchResults = new SearchResults('#all-results');
  const errorBox = new common.ErrorBox('#api-error-placeholder');

  const doSearch = (principal) => {
    if (isGroup(principal)) {
      integratedUIAlert.setLink(
        common.INTEGRATED_UI_GROUPS_ROOT + "/" + principal + "?tab=ancestors");
    } else {
      integratedUIAlert.setLink(INTEGRATED_UI_LOOKUP_ROOT);
    }

    if (!principal) {
      return;
    }

    // Set the search parameter for the principal in the URL.
    setCurrentPrincipalInURL(principal);

    searchBar.disableInteraction();
    if (searchBar.input.value != principal) {
      // Update the text in the search box in case the principal was set
      // through the URL.
      searchBar.input.value = principal;
    }

    searchResults.hide();
    errorBox.clearError();
    loadingBox.setLoadStatus(true);

    doLookup(principal)
      .then((response) => {
        searchResults.setLookupResults(response);
        searchResults.show();
      })
      .catch((err) => {
        errorBox.showError(`Looking up "${principal}" failed`, err.error);
      })
      .finally(() => {
        loadingBox.setLoadStatus(false);
        searchBar.enableInteraction();
      });
  }

  // Search when the principal in the URL is changed.
  onCurrentPrincipalInURLChange(doSearch);

  // Search when the search button is clicked.
  searchBar.btn.addEventListener('click', (e) => {
    doSearch(searchBar.input.value);
  });

  // Search when "Enter" is hit in the search box.
  searchBar.input.addEventListener('keyup', (e) => {
    if (e.keyCode == 13) {
      doSearch(searchBar.input.value);
    }
  });

  // Initial state depends on whether there was a principal in the URL.
  const currentPrincipal = getCurrentPrincipalInURL();
  if (currentPrincipal) {
    // The search bar will be enabled in doSearch's `finally` block.
    doSearch(currentPrincipal);
  } else {
    searchBar.enableInteraction();
  }
}
