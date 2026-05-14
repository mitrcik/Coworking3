// Tiny in-page modal that replaces window.confirm() for destructive form submits.
//
// Forms tagged with `data-confirm="…question…"` (optionally
// `data-confirm-title="…"` and `data-confirm-ok="…"`) will, on submit, show
// the modal in <main> and only submit if the user clicks the confirm button.
//
// One modal markup lives in layout.html. JS toggles it; CSS gives it the
// in-page styling so it does not feel like a browser-native dialog.
(function () {
  function ensureModal() {
    return document.getElementById("app-modal");
  }

  function openModal(message, title, okText) {
    var modal = ensureModal();
    if (!modal) return Promise.resolve(true);
    var titleEl = modal.querySelector("[data-modal-title]");
    var bodyEl = modal.querySelector("[data-modal-body]");
    var okBtn = modal.querySelector("[data-modal-ok]");
    var cancelBtn = modal.querySelector("[data-modal-cancel]");
    var backdrop = modal.querySelector("[data-modal-backdrop]");
    if (titleEl) titleEl.textContent = title || "Подтверждение";
    if (bodyEl) bodyEl.textContent = message || "";
    if (okBtn) okBtn.textContent = okText || "Подтвердить";

    modal.hidden = false;
    document.documentElement.classList.add("is-modal-open");

    return new Promise(function (resolve) {
      function done(value) {
        modal.hidden = true;
        document.documentElement.classList.remove("is-modal-open");
        if (okBtn) okBtn.removeEventListener("click", onOk);
        if (cancelBtn) cancelBtn.removeEventListener("click", onCancel);
        if (backdrop) backdrop.removeEventListener("click", onCancel);
        document.removeEventListener("keydown", onKey);
        resolve(value);
      }
      function onOk() { done(true); }
      function onCancel() { done(false); }
      function onKey(e) {
        if (e.key === "Escape") onCancel();
        else if (e.key === "Enter") onOk();
      }
      if (okBtn) okBtn.addEventListener("click", onOk);
      if (cancelBtn) cancelBtn.addEventListener("click", onCancel);
      if (backdrop) backdrop.addEventListener("click", onCancel);
      document.addEventListener("keydown", onKey);
      if (okBtn) okBtn.focus();
    });
  }

  // Expose so other scripts can prompt confirmations programmatically.
  window.appConfirm = openModal;

  // Hook into <form data-confirm="…">.
  document.addEventListener("submit", function (e) {
    var form = e.target;
    if (!(form instanceof HTMLFormElement)) return;
    var message = form.getAttribute("data-confirm");
    if (!message) return;
    if (form.dataset.confirmAccepted === "1") return; // already confirmed
    e.preventDefault();
    var title = form.getAttribute("data-confirm-title") || "Подтверждение";
    var okText = form.getAttribute("data-confirm-ok") || "Подтвердить";
    openModal(message, title, okText).then(function (ok) {
      if (!ok) return;
      form.dataset.confirmAccepted = "1";
      // requestSubmit preserves the submitter where supported; fall back to submit().
      if (typeof form.requestSubmit === "function") {
        form.requestSubmit();
      } else {
        form.submit();
      }
    });
  }, true);
})();
