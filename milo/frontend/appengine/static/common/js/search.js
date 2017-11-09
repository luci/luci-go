// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// A library for doing filtering with lists.


(function(window) {
  'use strict';

  var milo = window.milo || {};

  /**
   * Given a root node, hide all elements that don't match the query.  The
   * expected structure is:
   * <div data="text">
   *   <li data="text"></li>
   * </div>
   *
   * This searches data on both the div the li tags.
   */
  milo.filter = function(root, query) {
    console.log("Called with root: " + root);
    console.log("Called with query: " + query);
    // No query? Just show everything by removing the filter class on root.
    if (query == "") {
      $(root).removeClass("filter");
      return;
    }

    // Turn on filter mode.
    if (!$(root).hasClass()) {
      $(root).addClass("filter")
    }

    var visible_divs = new Set();
    var visible_lis = new Set();
    $(root).find("li[data*='" + query + "']").each(function() {
      visible_lis.add(this);
      visible_divs.add($(this).parent("div")[0]);
    });
    $(root).find("div[data*='" + query + "']").each(function() {
      visible_divs.add(this);
    });

		// Make sure that the existance of the visible class matches the membership
	  // we pre-caculated earlier.
    $(root).find("div").each(function(){
      if (visible_divs.has(this) == $(this).hasClass("hidden")) {
        $(this).toggleClass("hidden");
      }
    });
    $(root).find("li").each(function(){
      if (visible_lis.has(this) == $(this).hasClass("hidden")) {
        $(this).toggleClass("hidden");
      }
    });
  };

  /* Return the given url parameter from the current url */
  milo.getUrlParameter = function getUrlParameter(sParam) {
    var sPageURL = decodeURIComponent(window.location.search.substring(1)),
        sURLVariables = sPageURL.split('&'),
        sParameterName,
        i;

		for (i = 0; i < sURLVariables.length; i++) {
			sParameterName = sURLVariables[i].split('=');

			if (sParameterName[0] === sParam) {
					return sParameterName[1] === undefined ? true : sParameterName[1];
			}
		}
  };

  window.milo = milo;

}(window));

