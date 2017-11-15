// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// A library for doing filtering with lists.


(function(window) {
  'use strict';

  /**
   * Given a root node, hide all elements that don't match the query.  The
   * expected structure is:
   * <div>
   *   <div data-filterable="text">
   *     <li data-filterable="text"></li>
   *   </div>
   * </div>
   *
   * This searches the data tags under a root node.  For all elements that
   * match, all of its ancestors and descendants are marked visible.
   * The data-filterable attribute is expected to be in lowercase, and the
   * search is converted to lowercase.  This allows case insensitive search.
   * The current implementation uses jQuery selectors to power the search.
   *
   */

  const cache = new Map();
  function filter(root, query) {
    // No query? Make sure everything is visible, and then we're done here.
    if (!query) {
      root.find("[data-filterable]").show();
      return;
    }

    query = query.toLowerCase();

    // Create a jQuery selector for all words that are substrings.
    let selector = "*";
    for (const part of query.split(" ")) {
      if (part) {
        selector += "[data-filterable*='" + $.escapeSelector(part) + "']";
      }
    }

    // Make sure the element, all ancestors, and all descendants are visible.
    const visible_items = new Set();
    root.find(selector).each(function() {
      visible_items.add(this);
      $(this).parentsUntil(root, "[data-filterable]").each(function() {
        visible_items.add(this);
      });
      $(this).find("[data-filterable]").each(function() {
        visible_items.add(this);
      });
    });

    root.find("[data-filterable]").each(function() {
      // Hide the element if it is not in the visible items group.
      $(this).toggle(visible_items.has(this));
    });
  }

  /* Return the given url parameter from the current url */
  function getUrlParameter(param) {
    const pageURL = decodeURIComponent(window.location.search.substring(1));
    const components = pageURL.split('&');

    for (const component of components) {
      let [key, value] = component.split('=', 2);
      if (key === param) {
          return value === undefined ? "" : value;
      }
    }
    return ""
  }

  // Export all as module level functions.
  Object.assign(window.milo = window.milo || {}, {
    filter, getUrlParameter
  });

}(window));

