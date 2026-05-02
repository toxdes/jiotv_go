// tv.js - Spatial navigation and TV remote support for channel grid
// SPA mode: player opens as fullscreen overlay, back returns instantly to grid

(function () {
  "use strict";

  var grid = document.getElementById("tv-channel-grid");
  var noResults = document.getElementById("tv-no-results");
  var playerOverlay = document.getElementById("tv-player-overlay");
  var playerIframe = document.getElementById("tv-player-iframe");

  if (!grid) return;

  // ── Cached state ──────────────────────────────────────────────────────

  var allCards = [];
  var channelIdMap = Object.create(null);
  var visibleIndices = [];
  var cols = 5;
  var currentIndex = -1;
  var resizeTimer = null;
  var playerOpen = false;
  var lastFocusedIndex = -1;
  var playingCardIndex = -1;

  // ── SPA: player overlay ───────────────────────────────────────────────

  function openChannel(card) {
    var href = card.getAttribute("href");
    if (!href) return;

    // Find the real index of this card
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
    playerIframe.src = href;

    history.pushState({ overlay: true }, "", href);

    setTimeout(function () {
      playerIframe.focus();
    }, 200);
  }

  function closeChannel() {
    playerOpen = false;
    playerOverlay.classList.remove("active");
    document.body.style.overflow = "";
    playerIframe.src = "";

    history.replaceState(null, "", "/tv");

    setTimeout(function () {
      if (lastFocusedIndex >= 0 && allCards[lastFocusedIndex]) {
        focusCardByIndex(lastFocusedIndex);
      } else if (visibleIndices.length > 0) {
        focusCardByIndex(visibleIndices[0]);
      }
    }, 50);
  }

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
    channelIdMap = Object.create(null);
    for (var i = 0; i < cards.length; i++) {
      allCards.push(cards[i]);
      channelIdMap[cards[i].getAttribute("data-channel-id")] = cards[i];
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

  // ── Visible list management ────────────────────────────────────────────

  function rebuildVisible() {
    visibleIndices = [];
    for (var i = 0; i < allCards.length; i++) {
      if (!allCards[i].classList.contains("hidden")) {
        visibleIndices.push(i);
      }
    }
    if (noResults) {
      noResults.style.display = visibleIndices.length === 0 ? "" : "none";
    }
  }

  function indexInVisible(realIndex) {
    return visibleIndices.indexOf(realIndex);
  }

  // ── Click delegation for cards (SPA: open overlay instead of navigate) ─

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
    return visIndex;
  }

  function focusCardByIndex(realIndex) {
    var card = allCards[realIndex];
    if (!card) return;
    currentIndex = realIndex;
    card.focus({ preventScroll: true });
    card.scrollIntoView({ block: "nearest", behavior: "smooth" });
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
      // Find the currently playing card in the visible list
      var playVisIdx = indexInVisible(playingCardIndex);
      if (playVisIdx < 0) {
        // Current card may be hidden (fav filter), find nearest
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
      // On grid: move focus down/up a row
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

  // ── Favorites Toggle ────────────────────────────────────────────────────

  var favBtn = document.getElementById("tv-fav-btn");
  var favActive = false;
  var FAV_TOGGLE_KEY = "tvFavFilter";
  var FAV_CHANNELS_KEY = "favoriteChannels";

  function getFavIds() {
    try {
      var raw = localStorage.getItem(FAV_CHANNELS_KEY);
      return raw ? JSON.parse(raw) : [];
    } catch (e) {
      return [];
    }
  }

  function setFavActive(active) {
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
      // Unhide all cards and rebuild
      for (var i = 0; i < allCards.length; i++) {
        allCards[i].classList.remove("hidden");
      }
      rebuildVisible();
      currentIndex = -1;
      if (visibleIndices.length > 0) {
        focusCardByIndex(visibleIndices[0]);
      }
    }
  }

  function toggleFav() {
    setFavActive(!favActive);
  }

  function applyFavFilter() {
    var favIds = getFavIds();
    var favSet = Object.create(null);
    for (var i = 0; i < favIds.length; i++) {
      favSet[favIds[i]] = true;
    }
    for (var i = 0; i < allCards.length; i++) {
      if (!favSet[allCards[i].getAttribute("data-channel-id")]) {
        allCards[i].classList.add("hidden");
      }
    }
    rebuildVisible();
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

  // Restore fav toggle state from localStorage
  if (localStorage.getItem(FAV_TOGGLE_KEY) === "1") {
    setFavActive(true);
  }

  // ── Channel Number Typing ──────────────────────────────────────────────

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

  function resetNumBuffer() {
    numBuffer = "";
    clearTimeout(numTimer);
    numTimer = null;
    hideNumOverlay();
  }

  function commitNumBuffer() {
    if (numBuffer.length === 0) return;
    var targetId = numBuffer.replace(/^0+/, "") || "0";
    resetNumBuffer();

    var match = channelIdMap[targetId];
    if (match) {
      openChannel(match);
    } else if (numOverlay) {
      showNumOverlay("No match: " + targetId);
      setTimeout(hideNumOverlay, 1500);
    }
  }

  // ── Global Keyboard Handling ───────────────────────────────────────────

  document.addEventListener("keydown", function (e) {
    // ── SPA: Back key handling ───────────────────────────────────────
    if (e.key === "Backspace" || e.key === "GoBack" || e.keyCode === 461) {
      if (playerOpen) {
        e.preventDefault();
        closeChannel();
        return;
      }
    }

    // ── Channel Up/Down (works in grid and player) ────────────────────
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

    if (playerOpen) return; // let iframe handle all other keys

    // ── Grid keys ────────────────────────────────────────────────────

    // Digit keys — channel number typing
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
      if (nextVis >= 0 && nextVis < visibleIndices.length) {
        focusCardByIndex(visibleIndices[nextVis]);
      }
      return;
    }
  });

  // ── Initialization ──────────────────────────────────────────────────────

  var initFn = function () {
    initCache();
    if (visibleIndices.length > 0) {
      focusCardByIndex(visibleIndices[0]);
    }
  };

  if (window.requestIdleCallback) {
    requestIdleCallback(initFn, { timeout: 500 });
  } else {
    setTimeout(initFn, 100);
  }
})();
