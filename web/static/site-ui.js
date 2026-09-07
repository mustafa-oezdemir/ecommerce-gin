(() => {
  const refreshMarker = document.querySelector("[data-auto-refresh]");
  if (refreshMarker) {
    const seconds = Number(refreshMarker.dataset.autoRefresh);
    if (Number.isFinite(seconds) && seconds > 0) {
      window.setTimeout(() => window.location.reload(), seconds * 1000);
    }
  }

  "use strict";

  document.addEventListener("click", (event) => {
	const navigationToggle = event.target.closest("[data-nav-toggle]");
	if (navigationToggle) {
	  const navigation = navigationToggle.closest("[data-nav-menu]");
	  const open = navigation.classList.toggle("is-open");
	  navigationToggle.setAttribute("aria-expanded", String(open));
	  return;
	}
	for (const navigation of document.querySelectorAll("[data-nav-menu].is-open")) {
	  if (!navigation.contains(event.target)) {
		navigation.classList.remove("is-open");
		navigation.querySelector("[data-nav-toggle]")?.setAttribute("aria-expanded", "false");
	  }
	}
	for (const menu of document.querySelectorAll("details[data-overlay-menu][open]")) {
	  if (!menu.contains(event.target)) menu.removeAttribute("open");
	}
    const opener = event.target.closest("[data-dialog-open]");
    if (opener) {
      const dialog = document.getElementById(opener.dataset.dialogOpen);
      if (dialog instanceof HTMLDialogElement) dialog.showModal();
      return;
    }
    if (event.target instanceof HTMLDialogElement) {
      const bounds = event.target.getBoundingClientRect();
      const outside = event.clientX < bounds.left || event.clientX > bounds.right || event.clientY < bounds.top || event.clientY > bounds.bottom;
      if (outside) event.target.close();
    }
  });

  document.addEventListener("keydown", (event) => {
	if (event.key !== "Escape") return;
	for (const navigation of document.querySelectorAll("[data-nav-menu].is-open")) {
	  navigation.classList.remove("is-open");
	  navigation.querySelector("[data-nav-toggle]")?.setAttribute("aria-expanded", "false");
	}
	for (const menu of document.querySelectorAll("details[data-overlay-menu][open]")) menu.removeAttribute("open");
  });
})();
