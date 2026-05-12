// scheme.js — keeps /scheme reactive: form changes refresh seat statuses
// via /api/scheme without a full page reload (P.1), labels show the busy
// interval directly on the seat (P.2), and seat clicks update the form's
// hidden workspace_id input.
(function () {
  "use strict";

  var form = document.getElementById("scheme-form");
  if (!form) {
    return;
  }

  var dateInput = document.getElementById("scheme-date");
  var startInput = document.getElementById("scheme-start");
  var durationInputs = form.querySelectorAll('input[name="duration"]');
  var wsInput = document.getElementById("scheme-workspace-id");

  var errorBox = document.getElementById("scheme-error");
  var endDisplay = document.getElementById("scheme-end-display");
  var selectedCard = document.getElementById("scheme-selected-card");
  var bookForm = document.getElementById("scheme-book-form");
  var bookDisabled = document.getElementById("scheme-book-disabled");
  var historyLink = document.getElementById("scheme-history-link");

  var map = document.getElementById("scheme-map");

  // Debounced fetch so rapid changes (e.g. dragging the time picker) don't
  // hammer the API.
  var fetchTimer = null;
  function scheduleRefresh() {
    if (fetchTimer) {
      clearTimeout(fetchTimer);
    }
    fetchTimer = setTimeout(refresh, 150);
  }

  function selectedDuration() {
    for (var i = 0; i < durationInputs.length; i++) {
      if (durationInputs[i].checked) {
        return durationInputs[i].value;
      }
    }
    return "";
  }

  function refresh() {
    var params = new URLSearchParams();
    params.set("date", dateInput.value);
    params.set("start", startInput.value);
    params.set("duration", selectedDuration());
    if (wsInput.value) {
      params.set("workspace_id", wsInput.value);
    }
    fetch("/api/scheme?" + params.toString(), { credentials: "same-origin" })
      .then(function (resp) {
        if (!resp.ok) {
          throw new Error("scheme api failed: " + resp.status);
        }
        return resp.json();
      })
      .then(applyData)
      .catch(function (err) {
        console.error("scheme refresh failed:", err);
      });
  }

  function applyData(data) {
    if (data.error) {
      errorBox.textContent = data.error;
      errorBox.removeAttribute("hidden");
    } else {
      errorBox.textContent = "";
      errorBox.setAttribute("hidden", "");
    }
    if (endDisplay && data.end) {
      endDisplay.textContent = data.end;
    }
    renderMap(data.workspaces || [], data.selected);
    renderSelected(data);
  }

  function renderMap(seats, selected) {
    if (!map) return;
    var selID = selected && selected.id ? selected.id : null;
    map.innerHTML = "";
    for (var i = 0; i < seats.length; i++) {
      var s = seats[i];
      var node = document.createElement("a");
      node.className = "map__seat is-" + s.status_key + (selID === s.id ? " is-selected" : "");
      node.dataset.workspaceId = s.id;
      node.dataset.status = s.status_key;
      node.title = s.name + " — " + s.status_text + (s.busy_label ? " " + s.busy_label : "");
      node.href = "/scheme?workspace_id=" + encodeURIComponent(s.id);

      var nameEl = document.createElement("div");
      nameEl.className = "map__seat-name";
      nameEl.textContent = s.name;
      node.appendChild(nameEl);

      var typeEl = document.createElement("div");
      typeEl.className = "map__seat-type";
      typeEl.textContent = s.status_text;
      node.appendChild(typeEl);

      if (s.busy_label) {
        var labelEl = document.createElement("div");
        labelEl.className = "map__seat-label";
        labelEl.textContent = s.busy_label;
        node.appendChild(labelEl);
      }

      // Capture click so we can update the form without a full reload.
      node.addEventListener("click", function (id) {
        return function (ev) {
          ev.preventDefault();
          wsInput.value = id;
          history.replaceState(null, "", "/scheme?workspace_id=" + encodeURIComponent(id));
          scheduleRefresh();
        };
      }(s.id));
      map.appendChild(node);
    }
  }

  function renderSelected(data) {
    if (!selectedCard) return;
    if (!data.selected) {
      selectedCard.setAttribute("hidden", "");
      return;
    }
    selectedCard.removeAttribute("hidden");
    setText("scheme-selected-name", data.selected.name);
    setText("scheme-selected-type", data.selected.type);
    setText("scheme-selected-zone", data.selected.zone);
    setText("scheme-selected-date", data.date);
    setText("scheme-selected-start", data.start);
    setText("scheme-selected-end", data.end);
    setText("scheme-selected-status", data.selected.status_text);

    if (bookForm) {
      document.getElementById("scheme-book-workspace").value = data.selected.id;
      document.getElementById("scheme-book-date").value = data.date;
      document.getElementById("scheme-book-start").value = data.start;
      document.getElementById("scheme-book-end").value = data.end;
      var durEl = document.getElementById("scheme-book-duration");
      if (durEl) durEl.value = data.duration || "";
    }
    if (historyLink) {
      historyLink.href = "/workspaces/history?workspace_id=" +
        encodeURIComponent(data.selected.id) + "&filter=today";
    }
    if (data.can_book) {
      if (bookForm) bookForm.removeAttribute("hidden");
      if (bookDisabled) bookDisabled.setAttribute("hidden", "");
    } else {
      if (bookForm) bookForm.setAttribute("hidden", "");
      if (bookDisabled) bookDisabled.removeAttribute("hidden");
    }
  }

  function setText(id, value) {
    var el = document.getElementById(id);
    if (el) el.textContent = value || "";
  }

  // Highlight the currently checked duration chip.
  function refreshDurationChips() {
    for (var i = 0; i < durationInputs.length; i++) {
      var inp = durationInputs[i];
      var label = inp.closest(".duration-chip");
      if (!label) continue;
      if (inp.checked) {
        label.classList.add("is-active");
      } else {
        label.classList.remove("is-active");
      }
    }
  }

  // Wire up listeners.
  dateInput.addEventListener("change", scheduleRefresh);
  startInput.addEventListener("change", scheduleRefresh);
  startInput.addEventListener("input", scheduleRefresh);
  for (var i = 0; i < durationInputs.length; i++) {
    durationInputs[i].addEventListener("change", function () {
      refreshDurationChips();
      scheduleRefresh();
    });
  }

  // Stop the form's default GET submit so the page no longer reloads.
  form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    refresh();
  });

  // Initial map click delegation (for seats rendered server-side).
  if (map) {
    var initialSeats = map.querySelectorAll(".map__seat");
    for (var j = 0; j < initialSeats.length; j++) {
      (function (seat) {
        seat.addEventListener("click", function (ev) {
          ev.preventDefault();
          var id = seat.dataset.workspaceId;
          if (!id) return;
          wsInput.value = id;
          history.replaceState(null, "", "/scheme?workspace_id=" + encodeURIComponent(id));
          scheduleRefresh();
        });
      })(initialSeats[j]);
    }
  }

  // Auto-refresh every 30s so other users' bookings appear without action.
  setInterval(refresh, 30000);

  refreshDurationChips();
})();
