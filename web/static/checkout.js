document.addEventListener("DOMContentLoaded", () => {
  const form = document.querySelector("[data-checkout-form]");
  const billingToggle = document.querySelector("[data-billing-same]");
  const billingPanel = document.querySelector("[data-billing-panel]");
  const updateBilling = () => {
    if (!billingToggle || !billingPanel) return;
    billingPanel.hidden = billingToggle.checked;
    billingPanel.querySelectorAll("input").forEach((input) => {
      input.required = !billingToggle.checked && input.type === "radio";
    });
  };
  billingToggle?.addEventListener("change", updateBilling);
  updateBilling();
  form?.addEventListener("submit", () => {
    const button = form.querySelector("[data-place-order]");
    if (!button || button.disabled) return;
    button.disabled = true;
    button.textContent = "Processing...";
  });

  const paymentPoll = document.querySelector("[data-payment-poll]");
  if (paymentPoll) {
    const orderID = paymentPoll.dataset.orderId;
    let attempts = 0;
    const poll = async () => {
      attempts += 1;
      try {
        const response = await fetch(`/api/orders/${encodeURIComponent(orderID)}/payment-status`, {
          credentials: "same-origin",
          headers: { Accept: "application/json" },
          cache: "no-store",
        });
        if (response.ok) {
          const status = await response.json();
          if (status.payment_status !== "pending" && status.payment_status !== "authorized") {
            window.location.reload();
            return;
          }
        }
      } catch (_) {
        // A transient network failure must not turn a browser return into payment proof.
      }
      if (attempts < 30) window.setTimeout(poll, 2000);
    };
    window.setTimeout(poll, 1000);
  }
});
