// scheme.js — keeps /scheme reactive: form changes refresh seat statuses
// via /api/scheme without a full page reload, labels show the busy
// interval directly on the seat, seat clicks update the form's hidden
// workspace_id input.
//
// Adds:
//  - coworking combobox with substring search and grid resize on change;
//  - custom-interval mode (date + start + end, validated <= 3 h);
//  - cancellation cool-down banner rendering.
(function () {
  "use strict";

  var form = document.getElementById("scheme-form");
  if (!form) {
    return;
  }

  var dateInput = document.getElementById("scheme-date");
  var startInput = document.getElementById("scheme-start");
  var endInput = document.getElementById("scheme-end");
  var durationInputs = form.querySelectorAll('input[name="duration"]');
  var wsInput = document.getElementById("scheme-workspace-id");
  var cwInput = document.getElementById("scheme-coworking-id");
  var bookCwInput = document.getElementById("scheme-book-coworking");

  var errorBox = document.getElementById("scheme-error");
  var endDisplay = document.getElementById("scheme-end-display");
  var selectedCard = document.getElementById("scheme-selected-card");
  var bookForm = document.getElementById("scheme-book-form");
  var bookDisabled = document.getElementById("scheme-book-disabled");
  var historyLink = document.getElementById("scheme-history-link");
  var customError = document.getElementById("scheme-custom-error");

  var presetMode = document.getElementById("scheme-mode-preset");
  var customMode = document.getElementById("scheme-mode-custom");
  var tabPreset = document.getElementById("scheme-tab-preset");
  var tabCustom = document.getElementById("scheme-tab-custom");

  var combobox = document.getElementById("scheme-coworking");
  var comboInput = document.getElementById("scheme-coworking-input");
  var comboList = document.getElementById("scheme-coworking-list");
  var comboOptions = comboList ? comboList.querySelectorAll(".combobox__option") : [];

  var map = document.getElementById("scheme-map");
  var maxMinutes = parseInt(form.getAttribute("data-max-minutes"), 10) || 180;

  var mode = "preset"; // "preset" | "custom"

  // --- combobox ------------------------------------------------------------

  function openCombo() {
    if (!comboList) return;
    comboList.removeAttribute("hidden");
  }
  function closeCombo() {
    if (!comboList) return;
    comboList.setAttribute("hidden", "");
  }
  function filterCombo(query) {
    var needle = (query || "").toLowerCase().trim();
    var any = false;
    for (var i = 0; i < comboOptions.length; i++) {
      var opt = comboOptions[i];
      var name = (opt.getAttribute("data-name") || "").toLowerCase();
      var match = !needle || name.indexOf(needle) !== -1;
      if (match) {
        opt.removeAttribute("hidden");
        any = true;
      } else {
        opt.setAttribute("hidden", "");
      }
    }
    if (!any) {
      closeCombo();
    } else {
      openCombo();
    }
  }
  function chooseCoworking(opt) {
    if (!opt) return;
    var id = opt.getAttribute("data-id");
    var name = opt.getAttribute("data-name");
    if (!id || cwInput.value === id) {
      closeCombo();
      return;
    }
    cwInput.value = id;
    if (bookCwInput) bookCwInput.value = id;
    if (comboInput) comboInput.value = name;
    // New coworking → drop the previously selected workspace.
    wsInput.value = "";
    closeCombo();
    syncURL();
    scheduleRefresh();
  }
  if (comboInput) {
    comboInput.addEventListener("focus", function () { filterCombo(comboInput.value); });
    comboInput.addEventListener("input", function () { filterCombo(comboInput.value); });
    comboInput.addEventListener("blur", function () {
      // Delay so click on option fires first.
      setTimeout(closeCombo, 150);
    });
  }
  for (var k = 0; k < comboOptions.length; k++) {
    (function (opt) {
      opt.addEventListener("mousedown", function (ev) {
        ev.preventDefault();
        chooseCoworking(opt);
      });
    })(comboOptions[k]);
  }

  // --- mode tabs -----------------------------------------------------------

  function setMode(next) {
    mode = next;
    if (presetMode) presetMode.toggleAttribute("hidden", next !== "preset");
    if (customMode) customMode.toggleAttribute("hidden", next !== "custom");
    if (tabPreset) {
      tabPreset.classList.toggle("is-active", next === "preset");
      tabPreset.setAttribute("aria-selected", next === "preset" ? "true" : "false");
    }
    if (tabCustom) {
      tabCustom.classList.toggle("is-active", next === "custom");
      tabCustom.setAttribute("aria-selected", next === "custom" ? "true" : "false");
    }
    scheduleRefresh();
  }
  if (tabPreset) tabPreset.addEventListener("click", function () { setMode("preset"); });
  if (tabCustom) tabCustom.addEventListener("click", function () { setMode("custom"); });

  // --- refresh loop --------------------------------------------------------

  var fetchTimer = null;
  function scheduleRefresh() {
    if (fetchTimer) clearTimeout(fetchTimer);
    fetchTimer = setTimeout(refresh, 150);
  }

  function selectedDuration() {
    for (var i = 0; i < durationInputs.length; i++) {
      if (durationInputs[i].checked) return durationInputs[i].value;
    }
    return "";
  }

  function intervalMinutes(startStr, endStr) {
    if (!startStr || !endStr) return -1;
    var a = startStr.split(":");
    var b = endStr.split(":");
    if (a.length < 2 || b.length < 2) return -1;
    var s = parseInt(a[0], 10) * 60 + parseInt(a[1], 10);
    var e = parseInt(b[0], 10) * 60 + parseInt(b[1], 10);
    if (e <= s) e += 24 * 60;
    return e - s;
  }

  function validateCustom() {
    if (mode !== "custom") {
      if (customError) customError.setAttribute("hidden", "");
      return true;
    }
    var minutes = intervalMinutes(startInput.value, endInput && endInput.value);
    if (minutes <= 0) {
      if (customError) {
        customError.textContent = "Укажите время окончания позже времени начала.";
        customError.removeAttribute("hidden");
      }
      return false;
    }
    if (minutes > maxMinutes) {
      if (customError) {
        customError.textContent = "Максимальная длительность — " + (maxMinutes / 60) + " ч.";
        customError.removeAttribute("hidden");
      }
      return false;
    }
    if (customError) customError.setAttribute("hidden", "");
    return true;
  }

  function refresh() {
    var params = new URLSearchParams();
    if (cwInput && cwInput.value) params.set("coworking_id", cwInput.value);
    params.set("date", dateInput.value);
    params.set("start", startInput.value);
    if (mode === "preset") {
      params.set("duration", selectedDuration());
    } else if (endInput && endInput.value) {
      if (!validateCustom()) {
        return;
      }
      params.set("end", endInput.value);
    }
    if (wsInput.value) params.set("workspace_id", wsInput.value);

    fetch("/api/scheme?" + params.toString(), { credentials: "same-origin" })
      .then(function (resp) {
        if (!resp.ok) throw new Error("scheme api failed: " + resp.status);
        return resp.json();
      })
      .then(applyData)
      .catch(function (err) { console.error("scheme refresh failed:", err); });
  }

  function applyData(data) {
    if (data.error) {
      errorBox.textContent = data.error;
      errorBox.removeAttribute("hidden");
    } else {
      errorBox.textContent = "";
      errorBox.setAttribute("hidden", "");
    }
    if (endDisplay && data.end) endDisplay.textContent = data.end;
    if (endInput && data.end && mode === "preset") endInput.value = data.end;
    if (map && data.coworking) {
      map.style.setProperty("--cols", String(data.coworking.grid_cols));
      map.style.setProperty("--rows", String(data.coworking.grid_rows));
    }
    renderMap(data.workspaces || [], data.selected, data.empty_cells || []);
    renderSelected(data);
  }

  function renderMap(seats, selected, emptyCells) {
    if (!map) return;
    var selID = selected && selected.id ? selected.id : null;
    map.innerHTML = "";
    if (emptyCells) {
      for (var e = 0; e < emptyCells.length; e++) {
        var cell = document.createElement("div");
        cell.className = "map__cell-empty";
        cell.setAttribute("aria-hidden", "true");
        cell.style.gridColumn = String(emptyCells[e].x);
        cell.style.gridRow = String(emptyCells[e].y);
        map.appendChild(cell);
      }
    }
    for (var i = 0; i < seats.length; i++) {
      var s = seats[i];
      var node = document.createElement("a");
      node.className = "map__seat is-" + s.status_key + (selID === s.id ? " is-selected" : "");
      node.dataset.workspaceId = s.id;
      node.dataset.status = s.status_key;
      node.title = s.name + " — " + s.status_text + (s.busy_label ? " " + s.busy_label : "");
      node.href = "/scheme?workspace_id=" + encodeURIComponent(s.id);
      node.style.gridColumn = String(s.x);
      node.style.gridRow = String(s.y);

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

      node.addEventListener("click", function (id) {
        return function (ev) {
          ev.preventDefault();
          wsInput.value = id;
          syncURL();
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
      if (bookDisabled) bookDisabled.setAttribute("hidden", "");
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
      var elWs = document.getElementById("scheme-book-workspace");
      var elCw = document.getElementById("scheme-book-coworking");
      var elD = document.getElementById("scheme-book-date");
      var elS = document.getElementById("scheme-book-start");
      var elE = document.getElementById("scheme-book-end");
      var elDur = document.getElementById("scheme-book-duration");
      if (elWs) elWs.value = data.selected.id;
      if (elCw && data.coworking) elCw.value = data.coworking.id;
      if (elD) elD.value = data.date;
      if (elS) elS.value = data.start;
      if (elE) elE.value = data.end;
      if (elDur) elDur.value = mode === "preset" ? (data.duration || "") : "";
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

  function refreshDurationChips() {
    for (var i = 0; i < durationInputs.length; i++) {
      var inp = durationInputs[i];
      var label = inp.closest(".duration-chip");
      if (!label) continue;
      label.classList.toggle("is-active", inp.checked);
    }
  }

  function syncURL() {
    var params = new URLSearchParams();
    if (cwInput && cwInput.value) params.set("coworking_id", cwInput.value);
    if (wsInput && wsInput.value) params.set("workspace_id", wsInput.value);
    var qs = params.toString();
    history.replaceState(null, "", "/scheme" + (qs ? "?" + qs : ""));
  }

  // Listeners
  dateInput.addEventListener("change", scheduleRefresh);
  startInput.addEventListener("change", scheduleRefresh);
  startInput.addEventListener("input", scheduleRefresh);
  if (endInput) {
    endInput.addEventListener("change", function () {
      if (validateCustom()) scheduleRefresh();
    });
    endInput.addEventListener("input", function () {
      if (validateCustom()) scheduleRefresh();
    });
  }
  for (var i = 0; i < durationInputs.length; i++) {
    durationInputs[i].addEventListener("change", function () {
      refreshDurationChips();
      scheduleRefresh();
    });
  }

  form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    refresh();
  });

  // Initial map click delegation (server-rendered).
  if (map) {
    var initialSeats = map.querySelectorAll(".map__seat");
    for (var j = 0; j < initialSeats.length; j++) {
      (function (seat) {
        seat.addEventListener("click", function (ev) {
          ev.preventDefault();
          var id = seat.dataset.workspaceId;
          if (!id) return;
          wsInput.value = id;
          syncURL();
          scheduleRefresh();
        });
      })(initialSeats[j]);
    }
  }

  // Refresh every 30 s so other users' bookings show up without action.
  setInterval(refresh, 30000);

  refreshDurationChips();
})();
