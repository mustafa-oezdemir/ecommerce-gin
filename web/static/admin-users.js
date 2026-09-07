(() => {
  "use strict";

  document.querySelectorAll(".delete-user-form").forEach((form) => {
    form.addEventListener("submit", (event) => {
      const name = form.dataset.userName || "this user";
      if (!window.confirm(`Delete ${name}? This account will no longer be able to sign in.`)) {
        event.preventDefault();
      }
    });
  });
})();
