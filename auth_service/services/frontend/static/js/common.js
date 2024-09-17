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

const toComparableGroupName = (group) => {
  // Note: callerCanModify is optional; it can be undefined.
  const prefix = group.callerCanModify ? 'A' : 'B';
  const name = group.name;
  if (name.indexOf('/') != -1) {
    return prefix + 'C' + name;
  }
  if (name.indexOf('-') != -1) {
    return prefix + 'B' + name;
  }
  return prefix + 'A' + name;
}

const setMainNavbarActiveLink = () => {
  let path = window.location.pathname;

  // If listing a group, make the active link match the link for groups.
  if (path === '/auth/listing') {
    path = '/auth/groups';
  }

  document.querySelectorAll('#main-navbar li>a').forEach((link) => {
    const target = link.getAttribute('href');
    if (target && path.startsWith(target)) {
      link.classList.add('active');
      link.setAttribute('aria-current', 'page');
    } else {
      link.classList.remove('active');
      link.setAttribute('aria-current', null);
    }
  });
};

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
// Component to present an error which can be shown/hidden.
class ErrorBox extends HidableElement {
  constructor(container) {
    super(container, false);

    const template = document.querySelector('#error-box-template');
    const errorBox = template.content.cloneNode(true);

    this.title = errorBox.querySelector('#error-title');
    this.message = errorBox.querySelector('#error-message');

    this.element.append(errorBox);
  }

  #setError(title, message) {
    // Set the component's text to the given error title and message.
    this.title.replaceChildren(title);
    this.message.replaceChildren(message);
  }

  showError(title, message) {
    this.#setError(title, message);
    this.show();
  }

  clearError() {
    this.#setError('', '');
    this.hide();
  }
}


////////////////////////////////////////////////////////////////////////////////
// Component with centered spinner to represent loading.
class LoadingBox {
  constructor(parent) {
    const template = document.querySelector('#loading-box-template');
    const loadingBox = template.content.cloneNode(true);

    this.spinner = loadingBox.querySelector('#spinner');
    this.setLoadStatus(false);

    const parentElement = document.querySelector(parent);
    parentElement.appendChild(loadingBox);
  }

  setLoadStatus(isLoading) {
    this.spinner.style.display = isLoading ? 'block' : 'none';
  }
}


var common = (function () {
  'use strict';

  var exports = {};

  // Converts UTC timestamp (in microseconds) to a readable string in local TZ.
  exports.utcTimestampToString = (utc) => {
    const t = new Date(utc);
    let yyyy = `${t.getFullYear()}`
    yyyy = yyyy.padStart(4, '0');
    let mm = `${t.getMonth() + 1}`;
    mm = mm.padStart(2, '0');
    let dd = `${t.getDate()}`
    dd = dd.padStart(2, '0');
    return `${yyyy}-${mm}-${dd}, ${t.toLocaleTimeString()}`;
  };

  // Returns URL to a change log page for a given target.
  exports.getChangeLogTargetURL = (kind, name) => {
    return '/auth/change_log?target=' + encodeURIComponent(kind + '$' + name);
  };

  // Returns URL to a change log page for a given revision.
  exports.getChangeLogRevisionURL = (rev) => {
    return '/auth/change_log?auth_db_rev=' + encodeURIComponent('' + rev);
  };

  // Returns URL to the group page for a given group.
  exports.getGroupPageURL = (name) => {
    return '/auth/groups/' + encodeURIComponent(name);
  };

  // Returns URL for the given group's full listing page.
  exports.getGroupListingURL = (name) => {
    if (name) {
      return '/auth/listing?group=' + encodeURIComponent(name);
    }
    return '/auth/groups';
  };

  // Returns URL to a page with lookup results.
  exports.getLookupURL = (principal) => {
    if (principal) {
      return '/auth/lookup?p=' + encodeURIComponent(principal);
    }
    return '/auth/lookup';
  };

  // Returns value of URL query parameter given its name.
  exports.getQueryParameter = (name) => {
    // See http://stackoverflow.com/a/5158301.
    var match = new RegExp(
      '[?&]' + name + '=([^&]*)').exec(window.location.search);
    return match && decodeURIComponent(match[1].replace(/\+/g, ' '));
  };

  // Appends '<prefix>:' to a string if it doesn't have a prefix.
  exports.addPrefix = (prefix, str) => {
    return str.indexOf(':') == -1 ? prefix + ':' + str : str;
  };

  // Applies 'addPrefix' to each item of a list.
  exports.addPrefixToItems = (prefix, items) => {
    return items.map((item) => exports.addPrefix(prefix, item));
  };

  // Strips '<prefix>:' from a string if it starts with it.
  exports.stripPrefix = (prefix, str) => {
    if (!str) {
      return '';
    }
    if (str.slice(0, prefix.length + 1) == prefix + ':') {
      return str.slice(prefix.length + 1, str.length);
    } else {
      return str;
    }
  };

  // Applies 'stripPrefix' to each item of a list.
  exports.stripPrefixFromItems = (prefix, items) => {
    return items.map((item) => exports.stripPrefix(prefix, item));
  };

  // Returns sorted list of groups.
  //
  // Groups without '-' or '/' come first, then groups with '-'. Groups that can
  // be modified by a caller (based on 'caller_can_modify' field if available)
  // always come before read-only groups.
  exports.sortGroupsByName = (groups) => {
    return groups.toSorted((a, b) => {
      const aName = toComparableGroupName(a);
      const bName = toComparableGroupName(b);
      if (aName < bName) {
        return -1;
      } else if (aName > bName) {
        return 1;
      }
      return 0;
    });
  };

  exports.initializePage = () => {
    setMainNavbarActiveLink();

    // Enable the tooltip for the feedback link.
    const el = document.querySelector("#send-feedback");
    if (el) {
      new bootstrap.Tooltip(el);
    }
  };

  exports.HidableElement = HidableElement;
  exports.ErrorBox = ErrorBox;
  exports.LoadingBox = LoadingBox;

  return exports;
})();