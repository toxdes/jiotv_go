(() => {
  const script = document.currentScript;
  const video = document.getElementById("jiotv_go_player");
  if (!script || !video || !script.dataset.nextUrl) return;
  const target = new URL(script.dataset.nextUrl, window.location.origin);
  if (target.origin !== window.location.origin) return;
  const start = Number(script.dataset.start);
  const end = Number(script.dataset.end);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
    video.addEventListener("ended", () => window.location.replace(target.href), { once: true });
    return;
  }
  let serverTime = Number(script.dataset.serverTime);
  let clockSample = performance.now();
  let transitioning = false;
  let lastSeek = -Infinity;
  const now = () => serverTime + performance.now() - clockSample;
  const duration = () => Number.isFinite(video.duration) && video.duration > 0 ? video.duration : Number(script.dataset.duration);
  const offset = () => {
    const length = duration();
    if (!(length > 0)) return 0;
    const elapsed = Math.max(0, (now() - start) / 1000);
    const slot = (end - start) / 1000;
    const complete = Math.floor(slot / length) * length;
    let position = elapsed % length;
    if (script.dataset.padding === "true" || (slot >= length && complete < slot && elapsed >= complete)) {
      position = length - Math.max(0, (end - now()) / 1000);
    }
    return Math.max(0, Math.min(length - 0.05, position));
  };
  const requestStarted = performance.now();
  const clockRequest = fetch(script.dataset.clockUrl, { cache: "no-store" })
    .then((response) => { if (!response.ok) throw new Error("Clock unavailable"); return response.json(); })
    .then((data) => {
      if (!Number.isFinite(data.server_time)) return;
      serverTime = data.server_time + (performance.now() - requestStarted) / 2;
      clockSample = performance.now();
    }).catch(() => {});
  const ready = Promise.race([clockRequest, new Promise((resolve) => setTimeout(resolve, 1500))]);
  const transition = () => {
    if (transitioning) return;
    transitioning = true;
    if (window.parent !== window) window.parent.postMessage({ type: "plugin-vod-schedule-changed" }, window.location.origin);
    window.location.replace(target.href);
  };
  const seek = () => {
    if (now() >= end) { transition(); return; }
    const position = offset();
    if (Math.abs(video.currentTime - position) > 1) {
      lastSeek = performance.now();
      video.currentTime = position;
    }
  };
  const resume = () => {
    if (now() >= end) { transition(); return; }
    if (end - now() < 250) return;
    video.currentTime = offset();
    lastSeek = performance.now();
    const promise = video.play();
    if (promise) promise.catch(() => {});
  };
  let playbackReady = false;
  const timer = setInterval(() => {
    if (now() >= end) { transition(); return; }
    if (playbackReady && !video.paused && !video.seeking && video.readyState >= 3 && performance.now() - lastSeek > 15000 && Math.abs(video.currentTime - offset()) > 4) seek();
  }, 500);
  window.addEventListener("pagehide", () => clearInterval(timer), { once: true });
  window.pluginVODPlayback = {
    ready,
    offset,
    start() {
      playbackReady = true;
      seek();
      video.addEventListener("playing", seek, { once: true });
      video.addEventListener("ended", resume);
      document.addEventListener("visibilitychange", () => { if (!document.hidden) seek(); });

    },
  };
})();
