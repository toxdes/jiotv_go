// player_tv.js — JTV fork: TV remote support for player pages
// Only activates when loaded inside an iframe (TV mode).
// Desktop/standalone playback is completely unaffected.

(function () {
  "use strict";

  // Self-guard: only active in iframe (TV mode), skip on desktop
  if (window.parent === window) return;

  var video = null;
  var active = false;

  function findVideo() {
    if (video) return video;
    // Shaka Player: <video> is in the HTML
    video = document.querySelector("video");
    // Flowplayer: creates video inside #jiotv_go_player
    if (!video) {
      var container = document.getElementById("jiotv_go_player");
      if (container) video = container.querySelector("video");
    }
    return video;
  }

  function postToParent(msg) {
    try {
      window.parent.postMessage(msg, "*");
    } catch (e) {}
  }

  function togglePlay() {
    var v = findVideo();
    if (!v) return;
    if (v.paused) {
      v.play().catch(function () {});
    } else {
      v.pause();
    }
  }

  function doPlay() {
    var v = findVideo();
    if (v && v.paused) v.play().catch(function () {});
  }

  function doPause() {
    var v = findVideo();
    if (v && !v.paused) v.pause();
  }

  // ── Parent commands ────────────────────────────────────────────────

  window.addEventListener("message", function (e) {
    if (!e.data || typeof e.data.type !== "string") return;
    switch (e.data.type) {
      case "togglePlay":
        togglePlay();
        break;
      case "play":
        doPlay();
        break;
      case "pause":
        doPause();
        break;
    }
  });

  // ── Media keys (TV remote play/pause) ──────────────────────────────

  document.addEventListener("keydown", function (e) {
    if (
      e.key === "MediaPlayPause" ||
      e.keyCode === 179 ||
      e.keyCode === 415
    ) {
      e.preventDefault();
      togglePlay();
    }
  });

  // ── State → parent ─────────────────────────────────────────────────

  function onVideoReady() {
    if (active) return;
    active = true;
    var v = findVideo();
    if (v) {
      v.volume = 1.0;
      v.addEventListener("play", function () {
        postToParent({ type: "playing" });
      });
      v.addEventListener("pause", function () {
        postToParent({ type: "paused" });
      });
      // Handle autoplay failures — if video stays paused after load
      setTimeout(function () {
        if (v.paused) {
          postToParent({ type: "autoplayFailed" });
        }
      }, 3000);
    }
    postToParent({ type: "ready" });
  }

  // Signal ready after Shaka/Flowplayer has had time to initialize
  setTimeout(onVideoReady, 500);

  // ── Disarm player UI controls (prevent focus stealing) ─────────────

  function disarmControls() {
    var btns = document.querySelectorAll(
      '.shaka-controls-container [role="button"],' +
      '.shaka-controls-container button,' +
      '.fp-ui [role="button"],' +
      ".fp-play, .fp-pause"
    );
    for (var i = 0; i < btns.length; i++) {
      btns[i].setAttribute("tabindex", "-1");
    }
  }

  // Run repeatedly — Shaka/Flowplayer create controls asynchronously
  setInterval(disarmControls, 2000);
  setTimeout(disarmControls, 1000);

})();
