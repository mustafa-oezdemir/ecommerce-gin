(() => {
  "use strict";

  const token = document.querySelector('meta[name="csrf-token"]')?.content ?? "";

  document.querySelectorAll(".favorite").forEach((button) => {
    button.addEventListener("click", async () => {
      const active = button.dataset.active === "true";
      const label = button.querySelector("span");

      button.disabled = true;
      button.classList.toggle("active", !active);
      button.dataset.active = String(!active);
      button.setAttribute("aria-pressed", String(!active));
      if (label) label.textContent = active ? "Favorite" : "Favorited";

      try {
        const response = await fetch(`/products/${button.dataset.id}/favorite`, {
          method: active ? "DELETE" : "POST",
          headers: {
            "X-CSRF-Token": token,
            "X-Requested-With": "XMLHttpRequest",
            Accept: "application/json",
          },
        });
        if (!response.ok) throw new Error("Favorite could not be updated");
      } catch {
        button.classList.toggle("active", active);
        button.dataset.active = String(active);
        button.setAttribute("aria-pressed", String(active));
        if (label) label.textContent = active ? "Favorited" : "Favorite";
      } finally {
        button.disabled = false;
      }
    });
  });
})();
