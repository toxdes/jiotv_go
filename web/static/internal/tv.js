// tv.js - Spatial navigation and TV remote support for channel grid
// SPA mode: player opens as fullscreen overlay, back returns instantly to grid

(function () {
  "use strict";

  var grid = document.getElementById("tv-channel-grid");
  var noResults = document.getElementById("tv-no-results");
  var playerOverlay = document.getElementById("tv-player-overlay");
  var playerIframe = document.getElementById("tv-player-iframe");
  var chanLabel = document.getElementById("tv-player-chan-label");

  if (!grid) return;

  // ── Server fav IDs (from config.toml favorite_channel_ids) ────────────

  var serverFavIds = window.TV_FAVORITE_IDS || [];

  // ── Cached state ──────────────────────────────────────────────────────

  var allCards = [];
  var cardTitles = [];      // pre-cached .tv-card-title elements
  var cardIds = [];         // pre-cached data-channel-id strings
  var channelIdMap = Object.create(null);
  var visibleIndices = [];
  var visibleRank = [];     // visibleRank[realIndex] = position in visibleIndices, or -1
  var cols = 5;
  var currentIndex = -1;
  var resizeTimer = null;
  var playerOpen = false;
  var lastFocusedIndex = -1;
  var playingCardIndex = -1;

  // ── SPA: player overlay ───────────────────────────────────────────────

  function openChannel(card) {
    var id = card.getAttribute("data-channel-id");
    var name = card.getAttribute("data-channel-name") || id;
    // Use the server-rendered player URL (from data-player-url attribute)
    // Falls back to /mpd/:id if the attribute is missing
    var url = card.getAttribute("data-player-url") || "/mpd/" + id + "?q=auto";

    for (var i = 0; i < allCards.length; i++) {
      if (allCards[i] === card) {
        playingCardIndex = i;
        break;
      }
    }

    lastFocusedIndex = currentIndex;
    playerOpen = true;
    playerOverlay.classList.add("active");
    document.body.style.overflow = "hidden";
    if (chanLabel) chanLabel.textContent = name;
    playerIframe.src = url;

    history.pushState({ overlay: true }, "", "/tv/play/" + id + "?name=" + encodeURIComponent(name));

    setTimeout(function () {
      playerIframe.focus();
    }, 200);
  }

  function closeChannel() {
    playerOpen = false;
    playerOverlay.classList.remove("active");
    document.body.style.overflow = "";
    playerIframe.src = "";
    if (chanLabel) chanLabel.textContent = "";

    history.replaceState(null, "", "/tv");

    setTimeout(function () {
      if (lastFocusedIndex >= 0 && allCards[lastFocusedIndex]) {
        focusCardByIndex(lastFocusedIndex);
      } else if (visibleIndices.length > 0) {
        focusCardByIndex(visibleIndices[0]);
      }
    }, 50);
  }

  // Capture-phase Back handler — intercepts before iframe sees it
  document.addEventListener("keydown", function (e) {
    if (playerOpen && (e.key === "GoBack" || e.keyCode === 461)) {
      e.preventDefault();
      e.stopPropagation();
      closeChannel();
    }
  }, true);

  // Listen for back message from player iframe
  window.addEventListener("message", function (e) {
    if (e.data && e.data.type === "back" && playerOpen) {
      closeChannel();
    }
  });

  // Browser back button
  window.addEventListener("popstate", function (e) {
    if (playerOpen) {
      closeChannel();
    }
  });

  // ── Initialization ─────────────────────────────────────────────────────

  function initCache() {
    var cards = grid.querySelectorAll(".tv-card");
    allCards = [];
    cardTitles = [];
    cardIds = [];
    channelIdMap = Object.create(null);
    for (var i = 0; i < cards.length; i++) {
      allCards.push(cards[i]);
      channelIdMap[cards[i].getAttribute("data-channel-id")] = cards[i];
      cardTitles.push(cards[i].querySelector(".tv-card-title"));
      cardIds.push(cards[i].getAttribute("data-channel-id"));
    }
    updateColumnCount();
    rebuildVisible();
  }

  function updateColumnCount() {
    cols = getComputedStyle(grid).gridTemplateColumns.split(" ").length || 5;
  }

  window.addEventListener("resize", function () {
    clearTimeout(resizeTimer);
    resizeTimer = setTimeout(updateColumnCount, 200);
  });

  // ── Visible list + channel numbers (1-indexed) ────────────────────────

  function rebuildVisible() {
    visibleIndices = [];
    visibleRank = [];
    for (var i = 0; i < allCards.length; i++) {
      if (!allCards[i].classList.contains("hidden")) {
        visibleRank[i] = visibleIndices.length;
        visibleIndices.push(i);
        if (cardTitles[i]) cardTitles[i].textContent = String(visibleIndices.length);
      } else {
        visibleRank[i] = -1;
        if (cardTitles[i]) cardTitles[i].textContent = "";
      }
    }
    if (noResults) {
      noResults.style.display = visibleIndices.length === 0 ? "" : "none";
    }
  }

  function indexInVisible(realIndex) {
    return (realIndex >= 0 && realIndex < visibleRank.length) ? visibleRank[realIndex] : -1;
  }

  // ── Click delegation (SPA: open overlay instead of navigate) ───────────

  grid.addEventListener("click", function (e) {
    var card = e.target.closest(".tv-card");
    if (!card) return;
    e.preventDefault();
    openChannel(card);
  });

  grid.addEventListener("keydown", function (e) {
    if (e.key !== "Enter") return;
    var card = e.target.closest(".tv-card");
    if (!card) return;
    e.preventDefault();
    openChannel(card);
  });

  // ── Spatial Navigation ────────────────────────────────────────────────

  function getAdjacentCell(visIndex, direction) {
    var total = visibleIndices.length;
    if (total === 0) return visIndex;
    var row = Math.floor(visIndex / cols);
    var col = visIndex % cols;
    var lastRow = Math.floor((total - 1) / cols);
    var lastColInRow = Math.min(cols - 1, total - 1 - row * cols);

    switch (direction) {
      case "ArrowRight":
        if (col < lastColInRow) return Math.min(visIndex + 1, total - 1);
        break;
      case "ArrowLeft":
        if (col > 0) return Math.max(visIndex - 1, 0);
        break;
      case "ArrowDown":
        if (row < lastRow) return Math.min(visIndex + cols, total - 1);
        break;
      case "ArrowUp":
        if (row > 0) return Math.max(visIndex - cols, 0);
        break;
    }
    return -1;
  }

  function focusCardByIndex(realIndex) {
    var card = allCards[realIndex];
    if (!card) return;
    currentIndex = realIndex;
    card.focus({ preventScroll: true });
    card.scrollIntoView({ block: "center", behavior: "instant" });
  }

  function focusFirstVisible() {
    if (visibleIndices.length > 0) {
      focusCardByIndex(visibleIndices[0]);
    }
  }

  function getFocusedVisibleIndex() {
    if (currentIndex < 0) return -1;
    return indexInVisible(currentIndex);
  }

  // ── Channel Surfing (ChannelUp / ChannelDown) ──────────────────────────

  function surfChannel(direction) {
    if (visibleIndices.length === 0) return;

    if (playerOpen) {
      var playVisIdx = indexInVisible(playingCardIndex);
      if (playVisIdx < 0) {
        for (var i = 0; i < visibleIndices.length; i++) {
          if (visibleIndices[i] >= playingCardIndex) {
            playVisIdx = i;
            break;
          }
        }
        if (playVisIdx < 0) playVisIdx = 0;
      }

      var nextVis;
      if (direction === "up") {
        nextVis = playVisIdx > 0 ? playVisIdx - 1 : visibleIndices.length - 1;
      } else {
        nextVis = playVisIdx < visibleIndices.length - 1 ? playVisIdx + 1 : 0;
      }

      openChannel(allCards[visibleIndices[nextVis]]);
    } else {
      var visIdx = getFocusedVisibleIndex();
      if (visIdx < 0) {
        focusFirstVisible();
        return;
      }

      var nextVis;
      if (direction === "up") {
        nextVis = Math.max(0, visIdx - cols);
      } else {
        nextVis = Math.min(visibleIndices.length - 1, visIdx + cols);
      }
      focusCardByIndex(visibleIndices[nextVis]);
    }
  }

  // ── Favorites Toggle (from server config) ──────────────────────────────

  var favBtn = document.getElementById("tv-fav-btn");
  var favActive = false;
  var FAV_TOGGLE_KEY = "tvFavFilter";
  var hasFavorites = serverFavIds.length > 0;

  // Hide the fav button if no favorites are configured
  if (favBtn && !hasFavorites) {
    favBtn.style.display = "none";
  }

  function getFavIds() {
    return serverFavIds;
  }

  function setFavActive(active) {
    if (!hasFavorites) return;
    favActive = active;
    if (favBtn) {
      if (active) {
        favBtn.classList.add("active");
        favBtn.querySelector("svg").setAttribute("fill", "currentColor");
      } else {
        favBtn.classList.remove("active");
        favBtn.querySelector("svg").setAttribute("fill", "none");
      }
    }
    localStorage.setItem(FAV_TOGGLE_KEY, active ? "1" : "0");

    if (active) {
      applyFavFilter();
    } else {
      for (var i = 0; i < allCards.length; i++) {
        allCards[i].classList.remove("hidden");
        allCards[i].style.order = "";
      }
      rebuildVisible();
      currentIndex = -1;
      if (visibleIndices.length > 0) {
        focusCardByIndex(visibleIndices[0]);
      }
    }
  }

  function toggleFav() {
    if (!hasFavorites) return;
    setFavActive(!favActive);
  }

  function applyFavFilter() {
    var favIds = getFavIds();
    if (favIds.length === 0) return;

    // Build rank map: config order (fast lookup)
    var favRank = Object.create(null);
    for (var i = 0; i < favIds.length; i++) {
      favRank[favIds[i]] = i;
    }

    // Single pass: collect favs with their config rank, hide the rest
    var favEntries = [];
    for (var i = 0; i < allCards.length; i++) {
      var rank = favRank[cardIds[i]];
      if (rank !== undefined) {
        favEntries.push({ index: i, rank: rank });
      } else {
        allCards[i].classList.add("hidden");
      }
    }

    // Sort by config rank — only favs (e.g. 60 items), not 900+
    favEntries.sort(function (a, b) { return a.rank - b.rank; });

    // Build visibleIndices, assign 1-indexed numbers, set CSS order
    visibleIndices = [];
    visibleRank = [];
    for (var i = 0; i < allCards.length; i++) {
      visibleRank[i] = -1;
    }
    for (var i = 0; i < favEntries.length; i++) {
      var realIdx = favEntries[i].index;
      visibleRank[realIdx] = i;
      visibleIndices.push(realIdx);
      allCards[realIdx].style.order = i;
      if (cardTitles[realIdx]) cardTitles[realIdx].textContent = String(i + 1);
    }
    // Clear numbers + order on hidden cards
    for (var i = 0; i < allCards.length; i++) {
      if (allCards[i].classList.contains("hidden")) {
        if (cardTitles[i]) cardTitles[i].textContent = "";
        allCards[i].style.order = "";
      }
    }

    if (noResults) {
      noResults.style.display = visibleIndices.length === 0 ? "" : "none";
    }
    currentIndex = -1;
    if (visibleIndices.length > 0) {
      focusCardByIndex(visibleIndices[0]);
    }
  }

  if (favBtn) {
    favBtn.addEventListener("click", function (e) {
      e.preventDefault();
      toggleFav();
    });

    favBtn.addEventListener("keydown", function (e) {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        toggleFav();
      }
    });
  }

  // ── Channel Number Typing (1-indexed position, fallback to ID) ────────

  var numBuffer = "";
  var numTimer = null;
  var numOverlay = document.getElementById("tv-num-overlay");
  var NUM_TIMEOUT = 1500;

  function showNumOverlay(text) {
    if (!numOverlay) return;
    numOverlay.textContent = text;
    numOverlay.style.display = "";
  }

  function hideNumOverlay() {
    if (!numOverlay) return;
    numOverlay.style.display = "none";
  }

  function commitNumBuffer() {
    if (numBuffer.length === 0) return;
    var raw = numBuffer.replace(/^0+/, "") || "0";
    resetNumBuffer();

    // First try as 1-indexed position in the visible list
    var idx = parseInt(raw, 10) - 1;
    if (idx >= 0 && idx < visibleIndices.length) {
      openChannel(allCards[visibleIndices[idx]]);
      return;
    }

    // Fall back to channel ID lookup
    var match = channelIdMap[raw];
    if (match) {
      openChannel(match);
    } else if (numOverlay) {
      showNumOverlay("No match: " + raw);
      setTimeout(hideNumOverlay, 1500);
    }
  }

  function resetNumBuffer() {
    numBuffer = "";
    clearTimeout(numTimer);
    numTimer = null;
    hideNumOverlay();
  }

  // ── Global Keyboard Handling ───────────────────────────────────────────

  document.addEventListener("keydown", function (e) {
    // ── Yellow key toggles favorites ──────────────────────────────────
    if (e.key === "Yellow" || e.key === "F2" || e.keyCode === 459) {
      e.preventDefault();
      toggleFav();
      return;
    }

    // ── SPA: Back key handling (bubbling phase — for keyboard Backspace) ─
    if (e.key === "Backspace") {
      if (playerOpen) {
        e.preventDefault();
        closeChannel();
        return;
      }
    }

    // ── Channel Up/Down ──────────────────────────────────────────────
    if (e.key === "ChannelUp" || e.keyCode === 427) {
      e.preventDefault();
      surfChannel("up");
      return;
    }
    if (e.key === "ChannelDown" || e.keyCode === 428) {
      e.preventDefault();
      surfChannel("down");
      return;
    }

    if (playerOpen) return;

    // ── Fav button arrow navigation ───────────────────────────────────
    if (document.activeElement === favBtn) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        focusFirstVisible();
        return;
      }
      if (e.key === "ArrowUp" || e.key === "ArrowLeft" || e.key === "ArrowRight") {
        return;
      }
    }

    // ── Grid keys ────────────────────────────────────────────────────

    if (e.key >= "0" && e.key <= "9") {
      e.preventDefault();
      clearTimeout(numTimer);
      numBuffer += e.key;
      showNumOverlay(numBuffer);
      numTimer = setTimeout(commitNumBuffer, NUM_TIMEOUT);
      return;
    }

    if (e.key === "Backspace" && numBuffer.length > 0) {
      e.preventDefault();
      numBuffer = numBuffer.slice(0, -1);
      if (numBuffer.length === 0) {
        hideNumOverlay();
        clearTimeout(numTimer);
      } else {
        showNumOverlay(numBuffer);
        clearTimeout(numTimer);
        numTimer = setTimeout(commitNumBuffer, NUM_TIMEOUT);
      }
      return;
    }

    if (e.key === "Enter" && numBuffer.length > 0) {
      e.preventDefault();
      clearTimeout(numTimer);
      commitNumBuffer();
      return;
    }

    if (!visibleIndices.length) return;

    if (
      e.key === "ArrowRight" ||
      e.key === "ArrowLeft" ||
      e.key === "ArrowDown" ||
      e.key === "ArrowUp"
    ) {
      e.preventDefault();

      var visIdx = getFocusedVisibleIndex();
      if (visIdx < 0) {
        focusFirstVisible();
        return;
      }

      var nextVis = getAdjacentCell(visIdx, e.key);
      if (nextVis >= 0) {
        focusCardByIndex(visibleIndices[nextVis]);
      } else if (e.key === "ArrowUp" && favBtn) {
        favBtn.focus();
      }
      return;
    }
  });

  // ── Initialization ──────────────────────────────────────────────────────

  var initFn = function () {
    initCache();
    // Restore fav toggle state after cache is populated
    if (localStorage.getItem(FAV_TOGGLE_KEY) === "1") {
      setFavActive(true);
    } else if (visibleIndices.length > 0) {
      focusCardByIndex(visibleIndices[0]);
    }
  };

  if (window.requestIdleCallback) {
    requestIdleCallback(initFn, { timeout: 500 });
  } else {
    setTimeout(initFn, 100);
  }
})();
