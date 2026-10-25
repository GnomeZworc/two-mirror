(() => {
  const versionRoot = new URL(document.documentElement.dataset.content_root, window.location.href);
  const switcherUrl = new URL(DOCUMENTATION_OPTIONS.theme_switcher_json_url, versionRoot);

  const resolveLinks = (root) => {
    root.querySelectorAll(".version-switcher__menu a[href]").forEach((anchor) => {
      anchor.href = new URL(anchor.getAttribute("href"), switcherUrl).href;
    });
  };

  resolveLinks(document);
  new MutationObserver(() => resolveLinks(document)).observe(document.documentElement, {
    childList: true,
    subtree: true,
  });
})();
