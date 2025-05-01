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

const PATTERN_GROUP_CHANGELOG = new RegExp('^AuthGroup\\$(.+)$');

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

const TOOLTIP_FORMATTERS = new Map([
  ['GROUP_MEMBERS_ADDED', (change) => formatPrincipalsForTooltip(change.members)],
  ['GROUP_MEMBERS_REMOVED', (change) => formatPrincipalsForTooltip(change.members)],
  ['GROUP_GLOBS_ADDED', (change) => formatPrincipalsForTooltip(change.globs)],
  ['GROUP_GLOBS_REMOVED', (change) => formatPrincipalsForTooltip(change.globs)],

  ['GROUP_NESTED_ADDED', (change) => formatListForTooltip(change.nested)],
  ['GROUP_NESTED_REMOVED', (change) => formatListForTooltip(change.nested)],

  ['GROUP_DESCRIPTION_CHANGED', (change) => formatValueForTooltip(change.description)],
  ['GROUP_CREATED', (change) => formatValueForTooltip(change.description)],

  ['GROUP_OWNERS_CHANGED', (change) => formatValueForTooltip(`${change.oldOwners} \u2192 ${change.owners}`)]
]);

function formatValueForTooltip(value) {
  if (!value) {
    // No tooltip needed for an undefined/empty value.
    return null;
  }
  const content = document.createElement('div');
  content.classList.add('text-start');
  content.append(document.createTextNode(value));
  return content;
}

function formatListForTooltip(items) {
  const content = document.createElement('div');
  content.classList.add('text-start');

  items.forEach((item) => {
    const value = (item || '').trim();
    if (!value) {
      // No entry needed for an undefined/empty value.
      return;
    }
    const node = document.createElement('div');
    node.appendChild(document.createTextNode(value));
    content.appendChild(node);
  });

  return content;
}

function formatPrincipalsForTooltip(principals) {
  const names = common.stripPrefixFromItems('user', principals);
  return formatListForTooltip(names);
}

// Parse target string (e.g. 'AuthGroup$name') into components, adds a readable
// title and URL to a change log for the target.
const parseTarget = (t) => {
  var kind = t.split('$', 1)[0];
  var name = t.substring(kind.length + 1);

  // Recognize some known targets.
  var title = name;
  var targetURL = null;
  switch (kind) {
    case 'AuthGroup':
      targetURL = common.getGroupPageURL(name);
      break;
    case 'AuthIPWhitelist':
      targetURL = '/auth/ip_allowlists';
      break;
    case 'AuthIPWhitelistAssignments':
      title = 'IP allowlist assignment';
      targetURL = '/auth/ip_allowlists';
      break;
    case 'AuthGlobalConfig':
      title = 'Global config';
      break;
    case 'AuthRealmsGlobals':
      title = 'Realms config';
      break;
  }

  return {
    kind: kind,
    name: name,
    title: title,
    changeLogTargetURL: common.getChangeLogTargetURL(kind, name),
    targetURL: targetURL
  };
}

// Given an AuthDBChange object, returns text blob to show in "Change details"
// modal.
const authDBChangeToTextBlob = (change) => {
  const KNOWN_CHANGE_KEYS = [
    'changeType',
    'target',
    'authDbRev',
    'who',
    'when',
    'comment',
    'appVersion'
  ];

  var text = '';

  // First visit known keys present in all changes. That way they are always
  // on top in the text representation (and in predefined order).
  KNOWN_CHANGE_KEYS.forEach((key) => {
    var val = change[key];
    if (val) {
      text += key + ': ' + val + '\n';
    }
  });

  // Then visit the rest (in stable order).
  var keys = Object.keys(change);
  keys.sort();
  keys.forEach((key) => {
    if (KNOWN_CHANGE_KEYS.includes(key)) {
      return;
    }
    var val = change[key];
    if (val instanceof Array) {
      if (val.length) {
        text += key + ':\n';
        val.forEach((item) => {
          text += '  ' + item + '\n';
        })
      }
    } else if (val) {
      text += key + ': ' + val + '\n';
    }
  });

  return text;
}

////////////////////////////////////////////////////////////////////////////////
// Element for change log popup.
class ChangeLogModal {
  constructor(element) {
    // Root DOM element.
    this.element = document.querySelector(element);
    this.modal = new bootstrap.Modal(this.element);

    this.detailsSection = this.element.querySelector('#details-text');
  }

  showDetails(change) {
    this.detailsSection.textContent = authDBChangeToTextBlob(change);
    this.modal.show();
  }
}

////////////////////////////////////////////////////////////////////////////////
class ChangeLogContent extends common.HidableElement {
  constructor(element, target, revision, changeLogModal, loadingBox, errorBox) {
    super(element, false);

    // Templates to clone when constructing elements.
    this.headerTemplate = document.querySelector('#change-log-header-template');
    this.rowTemplate = document.querySelector('#change-log-row-template');

    // Element for change log header.
    this.header = this.element.querySelector('#change-log-header');
    // Element for alert of no change logs found.
    this.emptyAlert = this.element.querySelector('#change-log-empty-alert');
    // Element for the table of change logs.
    this.table = this.element.querySelector('#change-log-table');
    // Element for change log table body.
    this.tableBody = this.element.querySelector('#change-log-body');
    // Element for change log details pop-up.
    this.changeLogModal = changeLogModal;
    // Element for loading spinner.
    this.loadingBox = loadingBox;
    // Element for API error.
    this.errorBox = errorBox;

    // If set, limits change log queries to given target.
    this.target = target;
    // If set, limits change log queries to specific revision only.
    this.revision = revision;
    // Page size.
    this.pageSize = 15;
    // Page token for the current page.
    this.pageToken = null;
    // Page token for the next page.
    this.nextPageToken = null;
    // Stack of page tokens for previous pages.
    this.prevPageTokens = [];
    // True is UI is locked (when doing AJAX).
    this.locked = false;
    // Tooltips with change details shown on hover.
    this.tooltips = [];

    // Attach events to pager buttons.
    var pager = document.getElementById('change-log-pager');
    var prev = pager.querySelector('#prev');
    var next = pager.querySelector('#next');
    prev.addEventListener('click', () => {
      this.prevClicked();
    });
    next.addEventListener('click', () => {
      this.nextClicked();
    });
  }

  #setAlertVisibility(show) {
    console.log('alert visibility called with', show);
    this.emptyAlert.style.display = show ? 'block' : 'none';
  }

  #setTableVisibility(show) {
    console.log('table visibility called with', show);
    this.table.style.display = show ? 'table' : 'none';
  }

  // initialize sets the header and fetches the first page of change logs.
  initialize() {
    this.updateHeader();

    this.loadingBox.setLoadStatus(true);
    this.refetchChangeLogs({
      pageToken: this.pageToken,
      onSuccess: (response) => {
        this.setChangeLogList(response.changes);
        this.nextPageToken = response.nextPageToken;
        this.show();
      },
      onSettled: () => {
        this.loadingBox.setLoadStatus(false);
      },
    });
  }

  // Called when '<- Newer' button is clicked.
  prevClicked() {
    if (this.prevPageTokens.length <= 0 || this.locked) {
      return;
    }
    var prev = this.prevPageTokens[this.prevPageTokens.length - 1];
    this.refetchChangeLogs({
      pageToken: prev,
      onSuccess: (response) => {
        this.setChangeLogList(response.changes);
        this.prevPageTokens.pop();
        this.pageToken = prev;
        this.nextPageToken = response.nextPageToken;
      },
    });
  }

  // Called when 'Older ->' button is clicked.
  nextClicked() {
    if (!this.nextPageToken || this.locked) {
      return;
    }
    var next = this.nextPageToken;
    this.refetchChangeLogs({
      pageToken: next,
      onSuccess: (response) => {
        this.setChangeLogList(response.changes);
        this.prevPageTokens.push(this.pageToken);
        this.pageToken = next;
        this.nextPageToken = response.nextPageToken;
      },
    });
  }

  // Disables or enables pager buttons based on 'prevCursor' and 'nextCursor'.
  updatePagerButtons() {
    var pager = document.querySelector('#change-log-pager');
    var prev = pager.querySelector('#prev');
    var next = pager.querySelector('#next');

    var hasPrev = this.prevPageTokens.length > 0;
    prev.classList.toggle('disabled', !hasPrev);
    next.classList.toggle('disabled', !this.nextPageToken);
    pager.style.display = (hasPrev || this.nextPageToken) ? 'block' : 'none';
  }

  // Update change log page header.
  updateHeader() {
    // Clone and grab elements to modify.
    var clone = this.headerTemplate.content.cloneNode(true);
    var title = clone.querySelector('h3');
    var targetURL = clone.querySelector('a');
    var kind = clone.querySelector('small');

    // Modify contents and append to parent.
    var t;
    if (this.target) {
      t = parseTarget(this.target);
    } else {
      t = { title: 'Global Log' };
    }

    if (t.targetURL) {
      targetURL.href = t.targetURL;
      targetURL.textContent = t.title;
    } else {
      title.textContent = t.title;
    }

    if (this.revision) {
      title.textContent += ' for revision ' + this.revision;
    }

    if (t.kind) {
      kind.textContent = t.kind;
    }

    this.header.appendChild(clone);
  }

  // Fetches a page of change logs, using the given page token.
  // If successful, onSuccess is called with the response.
  // Handles an API error if one occurs.
  // Once the fetch promise is settled, onSettled is called.
  refetchChangeLogs({ pageToken, onSuccess = (response) => { }, onSettled = () => { } }) {
    this.lockUI();
    return api.changeLogs(this.target, this.revision, this.pageSize, pageToken)
      .then((response) => {
        // Normalize the response as empty values may not be set.
        const normalized = {
          changes: response.changes || [],
          nextPageToken: response.nextPageToken || null,
        };
        onSuccess(normalized);
      })
      .catch((err) => {
        this.hide();
        this.errorBox.showError('Listing change logs failed', err.error);
      })
      .finally(() => {
        onSettled();
        this.unlockUI();
        this.updatePagerButtons();
      });
  }

  // Update change log table content.
  setChangeLogList(logs) {
    const addElement = (log) => {
      // Clone and grab elements to modify.
      var clone = this.rowTemplate.content.cloneNode(true);
      var rev = clone.querySelector('td.change-log-rev a');
      const changeTypeCell = clone.querySelector('td.change-log-type');
      var type = changeTypeCell.querySelector('span');
      var when = clone.querySelector('td.change-log-when');
      var who = clone.querySelector('td.change-log-who');
      var target = clone.querySelector('td.change-log-target a');

      // Add a tooltip if there's a formatter for this change type.
      if (TOOLTIP_FORMATTERS.has(log.changeType)) {
        const formatter = TOOLTIP_FORMATTERS.get(log.changeType)
        const tooltip = new bootstrap.Tooltip(changeTypeCell, {
          container: 'body',
          title: () => this.locked ? null : formatter(log),
          html: true,
          placement: 'right',
          trigger: 'hover',
        });
        this.tooltips.push(tooltip);
      }

      // Modify contents and append to parent.
      var t = parseTarget(log.target);
      rev.href = common.getChangeLogRevisionURL(log.authDbRev);
      rev.textContent = 'r' + log.authDbRev;
      type.textContent = log.changeType;
      when.textContent = common.utcTimestampToString(log.when);
      who.textContent = common.stripPrefix('user', log.who);
      target.href = t.changeLogTargetURL;
      target.textContent = t.title;

      type.addEventListener('click', () => {
        this.presentChange(log);
      });

      this.tableBody.appendChild(clone);
    }

    this.tableBody.replaceChildren();
    this.tooltips.forEach(t => t.dispose());
    this.tooltips = [];

    if (logs.length > 0) {
      this.#setAlertVisibility(false);
      logs.map((log) => {
        addElement(log);
      });
      this.#setTableVisibility(true);
      return;
    }

    // Show the alert for empty results instead.
    this.#setTableVisibility(false);
    this.#setAlertVisibility(true);
  }

  // Shows a popup with details of a single change.
  presentChange(change) {
    if (this.locked) {
      return;
    }

    this.changeLogModal.showDetails(change);
  }

  // Locks UI actions before AJAX.
  lockUI() {
    this.locked = true;
  }

  // Unlocks UI actions after AJAX.
  unlockUI() {
    this.locked = false;
  }
}

window.onload = () => {
  const changeLogModal = new ChangeLogModal('#change-log-modal');
  const loadingBox = new common.LoadingBox('#loading-box-placeholder');
  const errorBox = new common.ErrorBox('#api-error-placeholder');

  // Parse the URL for the target and AuthDB revision parameters.
  const target = common.getQueryParameter('target');
  let authDbRev = common.getQueryParameter('auth_db_rev');
  if (authDbRev) {
    authDbRev = parseInt(authDbRev);
  }

  if (target) {
    // Add a link to the integrated UI if the target is an AuthGroup.
    const groupMatch = target.match(PATTERN_GROUP_CHANGELOG);
    if (groupMatch) {
      const integratedUIAlert = new common.IntegratedUIAlert('#integrated-ui-alert-container');
      integratedUIAlert.setLink(
        common.INTEGRATED_UI_GROUPS_ROOT + "/" + groupMatch[1] + "?tab=history");
    }
  }

  const changeLogContent = new ChangeLogContent(
    '#change-log-content', target, authDbRev, changeLogModal, loadingBox, errorBox);
  changeLogContent.initialize();
}
