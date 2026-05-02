// tv.js - Spatial navigation and TV remote support for channel grid

(function () {
  "use strict";

  var grid = document.getElementById("tv-channel-grid");
  var searchInput = document.getElementById("tv-search-input");
  var qualitySelect = document.getElementById("tv-quality-select");
  var noResults = document.getElementById("tv-no-results");

  if (!grid) return;

  // ── Spatial Navigation ────────────────────────────────────────────────

  function getCardElements() {
    return Array.from(grid.querySelectorAll(".tv-card"));
  }

  function getColumnCount() {
    var style = getComputedStyle(grid);
    var cols = style.gridTemplateColumns.split(" ").length;
    return cols || 5;
  }

  function getAdjacentCell(currentIndex, direction, total, cols) {
    var row = Math.floor(currentIndex / cols);
    var col = currentIndex % cols;
    var lastRow = Math.floor((total - 1) / cols);
    var lastColInRow = Math.min(cols - 1, total - 1 - row * cols);

    switch (direction) {
      case "ArrowRight":
        if (col < lastColInRow) return Math.min(currentIndex + 1, total - 1);
        break;
      case "ArrowLeft":
        if (col > 0) return Math.max(currentIndex - 1, 0);
        break;
      case "ArrowDown":
        if (row < lastRow) {
          var nextIdx = currentIndex + cols;
          return Math.min(nextIdx, total - 1);
        }
        break;
      case "ArrowUp":
        if (row > 0) return Math.max(currentIndex - cols, 0);
        break;
    }
    return currentIndex;
  }

  function scrollCardIntoView(card) {
    var cardRect = card.getBoundingClientRect();
    var headerHeight = 90;
    var margin = 20;

    if (cardRect.bottom > window.innerHeight - margin) {
      window.scrollBy({
        top: cardRect.bottom - window.innerHeight + margin,
        behavior: "smooth",
      });
    } else if (cardRect.top < headerHeight + margin) {
      window.scrollBy({
        top: cardRect.top - headerHeight - margin,
        behavior: "smooth",
      });
    }
  }

  function focusCard(card) {
    if (!card) return;
    card.focus({ preventScroll: true });
    scrollCardIntoView(card);
  }

  // ── Search ─────────────────────────────────────────────────────────────

  var searchTimeout = null;

  function filterChannels(query) {
    var cards = getCardElements();
    var q = query.toLowerCase().trim();
    var visibleCount = 0;

    cards.forEach(function (card) {
      var name = (card.getAttribute("data-channel-name") || "").toLowerCase();
      var id = (card.getAttribute("data-channel-id") || "").toLowerCase();

      if (!q || name.indexOf(q) !== -1 || id.indexOf(q) !== -1) {
        card.style.display = "";
        visibleCount++;
      } else {
        card.style.display = "none";
      }
    });

    if (noResults) {
      noResults.style.display = visibleCount === 0 ? "" : "none";
    }
  }

  if (searchInput) {
    searchInput.addEventListener("input", function () {
      filterChannels(this.value);
    });

    // When search input is focused, allow typing; Enter moves to first card
    searchInput.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown" || e.key === "Enter") {
        var cards = getCardElements();
        var visible = cards.filter(function (c) {
          return c.style.display !== "none";
        });
        if (visible.length > 0) {
          e.preventDefault();
          focusCard(visible[0]);
        }
      }
    });
  }

  // ── Quality Select ─────────────────────────────────────────────────────

  if (qualitySelect) {
    qualitySelect.addEventListener("change", function () {
      var quality = this.value;
      var cards = getCardElements();

      cards.forEach(function (card) {
        var href = card.getAttribute("href");
        if (!href) return;
        var url = new URL(href, window.location.origin);
        url.searchParams.set("q", quality);
        card.setAttribute("href", url.pathname + url.search);
      });
    });
  }

  // ── Channel Number Typing ──────────────────────────────────────────────

  var numBuffer = "";
  var numTimer = null;
  var numOverlay = document.getElementById("tv-num-overlay");
  var NUM_TIMEOUT = 1500; // clear buffer after 1.5s of no input

  function showNumOverlay(text) {
    if (!numOverlay) return;
    numOverlay.textContent = text;
    numOverlay.style.display = "";
  }

  function hideNumOverlay() {
    if (!numOverlay) return;
    numOverlay.style.display = "none";
    numOverlay.textContent = "";
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

    // Try to find and navigate to the matching channel
    var cards = getCardElements();
    for (var i = 0; i < cards.length; i++) {
      if (cards[i].getAttribute("data-channel-id") === targetId) {
        window.location.href = cards[i].getAttribute("href");
        return;
      }
    }

    // Not found — briefly show error
    if (numOverlay) {
      showNumOverlay("No match: " + targetId);
      setTimeout(hideNumOverlay, 1500);
    }
  }

  // ── Global Keyboard Handling ───────────────────────────────────────────

  document.addEventListener("keydown", function (e) {
    var tag = document.activeElement ? document.activeElement.tagName : "";

    // Don't intercept when typing in text inputs
    if (tag === "INPUT" || tag === "TEXTAREA") {
      if (tag === "INPUT" && e.key === "Escape") {
        document.activeElement.blur();
      }
      return;
    }

    // Channel number typing — intercept digit keys
    if (e.key >= "0" && e.key <= "9") {
      e.preventDefault();
      clearTimeout(numTimer);
      numBuffer += e.key;
      showNumOverlay(numBuffer);
      numTimer = setTimeout(commitNumBuffer, NUM_TIMEOUT);
      return;
    }

    // Backspace clears last digit in buffer
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

    // Enter commits the number buffer immediately
    if (e.key === "Enter" && numBuffer.length > 0) {
      e.preventDefault();
      clearTimeout(numTimer);
      commitNumBuffer();
      return;
    }

    var cards = getCardElements();
    var visible = cards.filter(function (c) {
      return c.style.display !== "none";
    });

    if (visible.length === 0) return;

    var current = document.activeElement;
    var isCard = current && current.classList.contains("tv-card");
    var cols = getColumnCount();
    var total = visible.length;

    if (
      e.key === "ArrowRight" ||
      e.key === "ArrowLeft" ||
      e.key === "ArrowDown" ||
      e.key === "ArrowUp"
    ) {
      e.preventDefault();

      if (!isCard) {
        // No card focused yet — focus the first visible one
        focusCard(visible[0]);
        return;
      }

      var currentIndex = visible.indexOf(current);
      if (currentIndex === -1) {
        focusCard(visible[0]);
        return;
      }

      var nextIndex = getAdjacentCell(currentIndex, e.key, total, cols);
      focusCard(visible[nextIndex]);
      return;
    }

    // Media play/pause key — toggle play on iframe if available
    if (
      e.key === "MediaPlayPause" ||
      e.keyCode === 179 ||
      e.keyCode === 415
    ) {
      var iframe = document.querySelector("iframe");
      if (iframe) {
        try {
          iframe.contentWindow.postMessage(
            { type: "togglePlay" },
            "*"
          );
        } catch (err) {
          // cross-origin, ignore
        }
      }
    }
  });

  // ── Initial Focus ──────────────────────────────────────────────────────

  document.addEventListener("DOMContentLoaded", function () {
    // Pre-focus the first card after a short delay
    setTimeout(function () {
      var cards = getCardElements();
      var visible = cards.filter(function (c) {
        return c.style.display !== "none";
      });
      if (visible.length > 0) {
        focusCard(visible[0]);
      } else if (searchInput) {
        searchInput.focus();
      }
    }, 300);
  });
})();
