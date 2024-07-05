// Copyright 2021 The LUCI Authors.
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

////////////////////////////////////////////////////////////////////////////////
// Selector is the dropdown selection menu containing allowlists.
class Selector {
  constructor(element) {
    // Root DOM element.
    this.element = document.getElementById(element);

    // The IP allowlist names from which to select.
    this.options = new Set();
  }

  select(name) {
    if (this.options.has(name)) {
      this.element.value = name;
      this.element.dispatchEvent(new Event('change'));
    }
  }

  populate(allowlists) {
    // Empty the element before populating.
    this.element.innerHTML = '';
    this.options.clear();

    let addToSelector = (name) => {
      if (this.options.has(name)) {
        return;
      }

      this.options.add(name);
      const option = document.createElement('option');
      option.text = name;
      option.setAttribute('value', name);
      this.element.appendChild(option);
    };

    // All allowlists, populate the selector element.
    allowlists.map((list) => {
      addToSelector(list.name);
    });
  }
}

////////////////////////////////////////////////////////////////////////////////
// AllowlistPane is the panel with information about some selected IP allowlist.
class AllowlistPane {
  constructor() {
    this.descriptionBox = document.getElementById('description-box');
    this.subnetBox = document.getElementById('subnet-box');
  }

  // Fills in the form with details about some IP allowlist.
  populate(ipAllowlist) {
    this.descriptionBox.innerHTML = '';
    this.subnetBox.innerHTML = '';
    this.descriptionBox.value = ipAllowlist.description;

    const subnets = ipAllowlist.subnets || [];
    this.subnetBox.textContent = subnets.join('\n');

    // Set the textarea's rows to be the number of subnets rounded up to the
    // nearest multiple of 5, up to a max of 25 rows. This reduces the need for
    // scrolling while still allowing for a lot of subnets.
    const rows = Math.min((Math.floor(subnets.length / 5) + 1)*5, 25);
    this.subnetBox.setAttribute('rows', rows);
  }
}


////////////////////////////////////////////////////////////////////////////////
// RevisionDetails is the inline text which specifies the IP allowlist
// config revision and view URL.
class RevisionDetails {
  constructor(element) {
    this.element = document.getElementById(element);
    this.linkElement = this.element.querySelector('a');
  }

  setLink(viewURL, revision) {
    this.linkElement.setAttribute('href', viewURL);
    this.linkElement.textContent = revision;
  }
}


window.onload = () => {
  const loadingBox = new common.LoadingBox('#loading-box-placeholder');

  const allowlistsContent = new common.HidableElement('#allowlists-content', false);
  const allowlistsConfig = new common.HidableElement('#allowlists-config', false);
  const emptyConfigFallback = new common.HidableElement('#empty-config-fallback', false);

  const errorBox = new common.ErrorBox('#api-error-placeholder');

  const selector = new Selector('allowlist-selector');
  const allowlistPane = new AllowlistPane();
  const revisionDetails = new RevisionDetails('revision-details');

  let ipAllowlists = new Map();
  // Add a listener to update the allowlist details based on the current
  // selection.
  selector.element.addEventListener('change', (event) => {
    const target = ipAllowlists.get(event.target.value);
    if (target) {
      allowlistPane.populate(target);
    }
  });

  const loadAllowlists = () => {
    errorBox.clearError();
    loadingBox.setLoadStatus(true);
    allowlistsContent.hide();
    allowlistsConfig.hide();
    emptyConfigFallback.hide();
    ipAllowlists.clear();

    return api.ipAllowlists()
    .then((response) => {
      const allowlists = response.allowlists || [];
      selector.populate(allowlists);

      if (allowlists.length === 0) {
        emptyConfigFallback.show();
      } else {
        // Set the IP allowlist data.
        response.allowlists.forEach((a) => {
          ipAllowlists.set(a.name, a);
        });

        // Select the first IP allowlist by default.
        const count = response.allowlists ? response.allowlists.length : 0;
        if (count > 0) {
          selector.select(response.allowlists[0].name);
        }
        allowlistsConfig.show();
      }

      revisionDetails.setLink(response.configViewUrl, response.configRevision);
      allowlistsContent.show();
    })
    .catch((err) => {
      errorBox.showError("Fetching IP allowlists failed", err.error);
    })
    .finally(() => {
      loadingBox.setLoadStatus(false);
    });
  };

  loadAllowlists();
};
