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
      targetURL = '/ip_allowlists';
      break;
    case 'AuthIPWhitelistAssignments':
      title = 'IP allowlist assignment';
      targetURL = '/ip_allowlists';
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

////////////////////////////////////////////////////////////////////////////////
class ChangeLogTable {
  constructor(headerElement, contentElement, modalElement, target, revision) {
    // Element for change log header.
    this.headerElement = document.getElementById(headerElement);
    // Element for change log table content.
    this.contentElement = document.getElementById(contentElement);
    // Element for change log popup.
    this.modalElement = document.getElementById(modalElement);
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
    })
    next.addEventListener('click', () => {
      this.nextClicked();
    })
  }

  // Refetches the current page.
  refresh() {
    var that = this;
    return this.refetchChangeLogs(this.pageToken, (response) => {
      that.setChangeLogList(response.changes);
      that.nextPageToken = response.nextPageToken;
    })
  }

  // Called when '<- Newer' button is clicked.
  prevClicked() {
    if (this.prevPageTokens.length <= 0 || this.locked) {
      return;
    }
    var prev = this.prevPageTokens[this.prevPageTokens.length - 1];
    var that = this;
    return this.refetchChangeLogs(prev, (response) => {
      that.setChangeLogList(response.changes);
      that.prevPageTokens.pop();
      that.pageToken = prev;
      that.nextPageToken = response.nextPageToken;
    })
  }

  // Called when 'Older ->' button is clicked.
  nextClicked() {
    if (!this.nextPageToken || this.locked) {
      return;
    }
    var next = this.nextPageToken;
    var that = this;
    return this.refetchChangeLogs(next, (response) => {
      that.setChangeLogList(response.changes);
      that.prevPageTokens.push(that.pageToken);
      that.pageToken = next;
      that.nextPageToken = response.nextPageToken;
    })
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
    if ('content' in document.createElement('template')) {
      var template = document.querySelector('#change-log-header-template')

      // Clone and grab elements to modify.
      var clone = template.content.cloneNode(true);
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

      this.headerElement.appendChild(clone);
    } else {
      // TODO: Find another way to add changeLogContent because the
      // HTML template element is not supported.
      console.error('Unable to load HTML template element, not supported.');
    }
  }

  // Loads list of change logs from a server.
  // Updates change log list UI. Returns deferred.
  refetchChangeLogs(pageToken, callback) {
    var that = this;
    var defer = api.changeLogs(this.target, this.revision, this.pageSize, pageToken);
    this.lockUI();
    defer
      .then((response) => {
        callback(response);
        that.unlockUI();
        this.updatePagerButtons();
      })
      .catch((err) => {
        console.log(err);
        that.unlockUI();
        this.updatePagerButtons();
      });
    return defer;
  }

  // Update change log table content.
  setChangeLogList(logs) {
    const addElement = (log) => {
      if ('content' in document.createElement('template')) {
        var template = document.querySelector('#change-log-row-template')

        // Clone and grab elements to modify.
        var clone = template.content.cloneNode(true);
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

        this.contentElement.appendChild(clone);
      } else {
        // TODO: Find another way to add changeLogContent because the
        // HTML template element is not supported.
        console.error('Unable to load HTML template element, not supported.');
      }
    }

    this.contentElement.replaceChildren();
    this.tooltips.forEach(t => t.dispose());
    this.tooltips = [];
    logs.map((log) => {
      addElement(log);
    });
  }

  // Shows a popup with details of a single change.
  presentChange(change) {
    // Given a change dict, returns text blob to show in "Change details" box.
    const changeToTextBlob = (change) => {
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
      // in top in the text representation (and in predefined order).
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

    if (this.locked) {
      return;
    }

    var details = this.modalElement.querySelector('#details-text');
    details.textContent = changeToTextBlob(change);

    var myModal = new bootstrap.Modal(this.modalElement);
    myModal.show();
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
  const loadingBox = new common.LoadingBox('#loading-box-placeholder');
  const changeLogContent = new common.HidableElement('#change-log-content', false);

  const target = common.getQueryParameter('target');
  let authDbRev = common.getQueryParameter('auth_db_rev');
  if (authDbRev) {
    authDbRev = parseInt(authDbRev);
  }

  const changeLogTable = new ChangeLogTable('change-log-header', 'change-log-body', 'change-log-details', target, authDbRev)
  changeLogTable.updateHeader();

  loadingBox.setLoadStatus(true);
  changeLogContent.hide();
  changeLogTable.refresh()
    .then(() => {
      changeLogContent.show();
    })
    .finally(() => {
      loadingBox.setLoadStatus(false);
    });
}
