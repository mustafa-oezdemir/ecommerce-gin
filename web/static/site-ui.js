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
})();
