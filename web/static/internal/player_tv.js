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
    video = document.querySelector("video");
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
    if (!video) findVideo();
    if (!video) return;
    if (video.paused) {
      video.play().catch(function () {});
    } else {
      video.pause();
    }
  }

  function doPlay() {
    if (!video) findVideo();
    if (video && video.paused) video.play().catch(function () {});
  }

  function unmuteVideo() {
    if (!video) findVideo();
    if (!video || !video.muted) return;

    video.volume = 1.0;
    video.muted = false;
  }

  function doPause() {
    if (!video) findVideo();
    if (video && !video.paused) video.pause();
  }

  function isTV2Frame() {
    try {
      return window.parent !== window && window.parent.location.pathname === "/tv2";
    } catch (e) {
      return false;
    }
  }

  function sendAudioTracks() {
    if (!isTV2Frame() || !window.jiotvTV2Audio) return;
    postToParent({ type: "audioTracks", tracks: window.jiotvTV2Audio.getTracks() || [] });
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
      case "unmute":
        unmuteVideo();
        break;
      case "pause":
        doPause();
        break;
      case "getAudioTracks":
        sendAudioTracks();
        break;
      case "selectAudioTrack":
        if (isTV2Frame() && window.jiotvTV2Audio) {
          window.jiotvTV2Audio.select(e.data.id);
          setTimeout(sendAudioTracks, 100);
        }
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
    findVideo();
    if (video) {
      video.volume = 1.0;
      video.addEventListener("play", function () {
        postToParent({ type: "playing", muted: video.muted });
      });
      video.addEventListener("pause", function () {
        postToParent({ type: "paused" });
      });
      video.addEventListener("volumechange", function () {
        postToParent({ type: "audioState", muted: video.muted });
      });
      setTimeout(function () {
        if (video.paused) {
          postToParent({ type: "autoplayFailed" });
        }
      }, 3000);
    }
    if (video) postToParent({ type: "audioState", muted: video.muted });
    postToParent({ type: "ready" });
  }

  setTimeout(onVideoReady, 500);

  // ── Disarm player UI controls (MutationObserver, not setInterval) ──

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

  var disarmTimer = null;
  function scheduleDisarm() {
    clearTimeout(disarmTimer);
    disarmTimer = setTimeout(disarmControls, 300);
  }

  if (window.MutationObserver) {
    var observer = new MutationObserver(scheduleDisarm);
    observer.observe(document.body, { childList: true, subtree: true });
  }
  // Initial run
  setTimeout(disarmControls, 1000);
  setTimeout(disarmControls, 3000);

})();
