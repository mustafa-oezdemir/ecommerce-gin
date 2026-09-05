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
});
