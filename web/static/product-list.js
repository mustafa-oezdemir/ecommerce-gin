(() => {
  "use strict";

  document.addEventListener("alpine:init", () => {
    window.Alpine.data("productFilters", () => ({
      open: false,
      draft: {},
      openPanel() {
        this.$refs.form.reset();
        this.captureDraft();
        this.open = true;
        document.body.classList.add("filter-drawer-open");
        this.$nextTick(() => this.$refs.closeButton.focus());
      },
      closePanel() {
        this.$refs.form.reset();
        this.captureDraft();
        this.open = false;
        document.body.classList.remove("filter-drawer-open");
        this.$nextTick(() => this.$refs.trigger.focus());
      },
      clearDraft() {
        for (const element of this.$refs.form.elements) {
          if (element.type === "hidden") {
            continue;
          }
          if (element.type === "checkbox" || element.type === "radio") {
            element.checked = false;
          } else if (element.tagName === "SELECT") {
            element.selectedIndex = 0;
          } else if (element.name && element.type !== "submit") {
            element.value = "";
          }
        }
        this.captureDraft();
      },
      captureDraft() {
        const draft = {};
        for (const [key, value] of new FormData(this.$refs.form).entries()) {
          if (Object.hasOwn(draft, key)) {
            draft[key] = Array.isArray(draft[key]) ? [...draft[key], value] : [draft[key], value];
          } else {
            draft[key] = value;
          }
        }
        this.draft = draft;
      },
      prepareSubmit() {
        for (const element of this.$refs.form.elements) {
          if (element.type !== "hidden" && element.name && typeof element.value === "string" && element.value.trim() === "") {
            element.disabled = true;
          }
        }
        this.captureDraft();
      },
    }));
  });

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

  document.querySelector(".product-sort")?.addEventListener("change", (event) => {
    const url = new URL(window.location.href);
    url.searchParams.set("sort", event.currentTarget.value);
    url.searchParams.delete("page");
    window.location.assign(url);
  });
})();
