(() => {
  const script = document.currentScript;
  const nextURL = script && script.dataset.nextUrl;
  const video = document.getElementById("jiotv_go_player");
  if (!nextURL || !video) return;
  const target = new URL(nextURL, window.location.origin);
  if (target.origin !== window.location.origin) return;
  video.addEventListener("ended", () => window.location.replace(target.href), { once: true });
})();
