(() => {
  "use strict";

  document.querySelector("#delete-account-form")?.addEventListener("submit", (event) => {
    if (!window.confirm("Permanently delete your account?")) {
      event.preventDefault();
    }
  });
})();
