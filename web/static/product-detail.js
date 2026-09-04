(() => {
  "use strict";

  const root = document.querySelector("#product-detail");
  if (!root) return;

  const productID = root.dataset.productId;
  const token = document.querySelector('meta[name="csrf-token"]')?.content ?? "";

  const request = async (url, method, body = null) => {
    const response = await fetch(url, {
      method,
      headers: {
        "X-CSRF-Token": token,
        "X-Requested-With": "XMLHttpRequest",
        Accept: "application/json",
      },
      body,
    });
    const text = await response.text();
    let data = {};
    if (text) {
      try {
        data = JSON.parse(text);
      } catch {
        data = {};
      }
    }
    if (!response.ok) {
      throw new Error(data?.error?.message || data?.message || "Request failed");
    }
    return data;
  };

  const showMessage = (text, isError = false) => {
    const element = document.querySelector("#message");
    if (!element) return;
    element.textContent = text;
    element.className = `alert ${isError ? "alert-danger" : "alert-success"}`;
  };

  const updateSummary = (summary) => {
    if (!summary) return;
    const values = {
      "#score-side": Number(summary.Average).toFixed(1),
      "#score-main": Number(summary.Average).toFixed(1),
      "#review-count": summary.Count,
      "#review-count-main": summary.Count,
    };
    Object.entries(values).forEach(([selector, value]) => {
      const element = document.querySelector(selector);
      if (element) element.textContent = value;
    });
  };

  const wireReview = (article) => {
    const deleteButton = article.querySelector(".delete-review");
    const editButton = article.querySelector(".edit-review");

    deleteButton?.addEventListener("click", async () => {
      if (!window.confirm("Delete this review?")) return;
      deleteButton.disabled = true;
      try {
        const result = await request(`/reviews/${article.dataset.review}`, "DELETE");
        article.remove();
        updateSummary(result?.data?.summary);
        showMessage("Review deleted.");
      } catch (error) {
        deleteButton.disabled = false;
        showMessage(error.message, true);
      }
    });

    editButton?.addEventListener("click", async () => {
      const rating = window.prompt("Rating (1–10)", article.dataset.rating);
      if (rating === null) return;
      const title = window.prompt("Review title", article.dataset.title);
      if (title === null) return;
      const body = window.prompt("Review", article.dataset.body);
      if (body === null) return;

      const form = new FormData();
      form.set("rating", rating);
      form.set("title", title);
      form.set("body", body);
      editButton.disabled = true;

      try {
        const result = await request(`/reviews/${article.dataset.review}`, "PUT", form);
        const data = result.data;
        article.dataset.rating = data.rating;
        article.dataset.title = data.title;
        article.dataset.body = data.body;
        article.querySelector(".review-rating").textContent = data.rating;
        article.querySelector(".review-title").textContent = data.title;
        article.querySelector(".review-body").textContent = data.body;
        updateSummary(data.summary);
        showMessage("Review updated.");
      } catch (error) {
        showMessage(error.message, true);
      } finally {
        editButton.disabled = false;
      }
    });
  };

  const createReview = (data) => {
    document.querySelector("#no-reviews")?.remove();
    const article = document.createElement("article");
    article.className = "review py-3";
    article.dataset.review = data.id;
    article.dataset.rating = data.rating;
    article.dataset.title = data.title;
    article.dataset.body = data.body;

    const top = document.createElement("div");
    top.className = "d-flex justify-content-between gap-3";
    const info = document.createElement("div");
    const title = document.createElement("strong");
    title.className = "review-title";
    title.textContent = data.title;
    const badge = document.createElement("span");
    badge.className = "badge text-bg-success ms-1";
    badge.textContent = "Verified Purchase";
    const stars = document.createElement("div");
    stars.className = "stars";
    stars.append("★ ");
    const rating = document.createElement("span");
    rating.className = "review-rating";
    rating.textContent = data.rating;
    stars.append(rating, "/10");
    info.append(title, badge, stars);
    const author = document.createElement("small");
    author.className = "text-muted";
    author.textContent = data.author;
    top.append(info, author);

    const body = document.createElement("p");
    body.className = "review-body mt-2 mb-1";
    body.textContent = data.body;
    const edit = document.createElement("button");
    edit.type = "button";
    edit.className = "btn btn-sm btn-outline-primary edit-review";
    edit.dataset.id = data.id;
    edit.textContent = "Edit";
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "btn btn-sm btn-outline-danger delete-review ms-1";
    remove.dataset.id = data.id;
    remove.textContent = "Delete";
    article.append(top, body, edit, remove);
    document.querySelector("#reviews")?.prepend(article);
    wireReview(article);
  };

  document.querySelector("#review-sort")?.addEventListener("change", (event) => {
    event.currentTarget.form?.requestSubmit();
  });

  const favorite = document.querySelector("#favorite");
  favorite?.addEventListener("click", async () => {
    const active = favorite.dataset.active === "true";
    const label = favorite.querySelector("span");
    favorite.disabled = true;
    favorite.dataset.active = String(!active);
    favorite.setAttribute("aria-pressed", String(!active));
    favorite.classList.toggle("active", !active);
    if (label) label.textContent = active ? "Add to favorites" : "Favorited";
    try {
      await request(`/products/${productID}/favorite`, active ? "DELETE" : "POST");
    } catch (error) {
      favorite.dataset.active = String(active);
      favorite.setAttribute("aria-pressed", String(active));
      favorite.classList.toggle("active", active);
      if (label) label.textContent = active ? "Favorited" : "Add to favorites";
      showMessage(error.message, true);
    } finally {
      favorite.disabled = false;
    }
  });

  document.querySelector("#list-form")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = event.currentTarget.querySelector('button[type="submit"]');
    if (button) button.disabled = true;
    try {
      await request(`/products/${productID}/lists`, "POST", new FormData(event.currentTarget));
      showMessage("Product added to your list.");
    } catch (error) {
      showMessage(error.message, true);
    } finally {
      if (button) button.disabled = false;
    }
  });

  document.querySelector("#review-form")?.addEventListener("submit", async (event) => {
    event.preventDefault();
    const form = event.currentTarget;
    const button = form.querySelector('button[type="submit"]');
    if (button) button.disabled = true;
    try {
      const result = await request(`/products/${productID}/reviews`, "POST", new FormData(form));
      createReview(result.data);
      updateSummary(result.data.summary);
      form.remove();
      showMessage("Review published.");
    } catch (error) {
      showMessage(error.message, true);
      if (button) button.disabled = false;
    }
  });

  document.querySelectorAll("[data-review]").forEach(wireReview);
})();
