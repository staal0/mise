"use strict";

const T = window.T || {};

// ---------- Interface language: cookie read by the server ----------
(function () {
  const btn = document.getElementById("lang");
  if (!btn) return;
  btn.addEventListener("click", () => {
    document.cookie = `lang=${btn.dataset.next}; path=/; max-age=31536000; SameSite=Lax`;
    location.reload();
  });
})();

// ---------- Theme: auto (follow system) → light → dark ----------
(function () {
  const btn = document.getElementById("theme");
  if (!btn) return;
  const order = ["auto", "light", "dark"];
  const labels = { auto: "◐ " + T.themeAuto, light: "☀ " + T.themeLight, dark: "☾ " + T.themeDark };
  let mode = document.documentElement.dataset.theme || "auto";

  function show() {
    btn.textContent = labels[mode];
    btn.setAttribute("aria-label", `${T.themeTitle}: ${labels[mode].slice(2)}`);
  }
  btn.addEventListener("click", () => {
    mode = order[(order.indexOf(mode) + 1) % order.length];
    if (mode === "auto") delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = mode;
    try {
      if (mode === "auto") localStorage.removeItem("theme");
      else localStorage.setItem("theme", mode);
    } catch (_) { /* storage blocked: choice lasts for this page only */ }
    show();
  });
  show();
})();

// ---------- Index: search + tag filter ----------
(function () {
  const cards = document.getElementById("cards");
  if (!cards) return;
  const search = document.getElementById("search");
  const tagNav = document.getElementById("tags");
  const nomatch = document.getElementById("nomatch");
  let activeTag = decodeURIComponent(location.hash.slice(1));

  function apply() {
    const words = search.value.toLowerCase().split(/\s+/).filter(Boolean);
    let shown = 0;
    for (const li of cards.children) {
      if (!li.dataset.search) continue;
      const tags = li.dataset.tags.split("|");
      const ok = words.every((w) => li.dataset.search.includes(w)) &&
        (!activeTag || tags.includes(activeTag));
      li.hidden = !ok;
      if (ok) shown++;
    }
    nomatch.hidden = shown > 0 || !cards.querySelector("[data-search]");
    if (tagNav) {
      for (const b of tagNav.children) b.classList.toggle("on", b.dataset.tag === activeTag);
    }
  }

  search.addEventListener("input", apply);
  if (tagNav) {
    tagNav.addEventListener("click", (e) => {
      const b = e.target.closest("[data-tag]");
      if (!b) return;
      activeTag = activeTag === b.dataset.tag ? "" : b.dataset.tag;
      history.replaceState(null, "", activeTag ? "#" + encodeURIComponent(activeTag) : location.pathname);
      apply();
    });
  }
  apply();
})();

// ---------- Recipe: "Read more" for long descriptions ----------
(function () {
  const desc = document.getElementById("desc");
  const more = document.getElementById("desc-more");
  if (!desc || !more) return;

  function check() {
    if (!desc.classList.contains("clamp")) return;
    more.hidden = desc.scrollHeight <= desc.clientHeight + 1;
  }
  more.addEventListener("click", () => {
    const open = desc.classList.toggle("clamp") === false;
    more.textContent = open ? T.showLess : T.readMore;
  });
  window.addEventListener("resize", check);
  check();
})();

// ---------- Recipe: scaling ----------
(function () {
  const scaler = document.getElementById("scaler");
  if (!scaler) return;
  const label = document.getElementById("scale-label");
  const base = parseFloat(scaler.dataset.base) || 0;
  let servings = base;
  let factor = 1;

  const fracs = [[0, ""], [1 / 8, "⅛"], [1 / 4, "¼"], [1 / 3, "⅓"], [1 / 2, "½"], [2 / 3, "⅔"], [3 / 4, "¾"], [1, ""]];

  function fmt(v, style) {
    if (style === "frac") {
      let whole = Math.floor(v);
      const rest = v - whole;
      let best = fracs[0];
      for (const f of fracs) if (Math.abs(f[0] - rest) < Math.abs(best[0] - rest)) best = f;
      if (Math.abs(best[0] - rest) < 0.04) {
        if (best[0] === 1) whole++;
        const s = (whole ? String(whole) : "") + best[1];
        return s || "0";
      }
    }
    const digits = v >= 100 ? 0 : v >= 10 ? 1 : 2;
    let s = String(Number(v.toFixed(digits)));
    if (style === "comma") s = s.replace(".", ",");
    return s;
  }

  function update() {
    for (const el of document.querySelectorAll(".q")) {
      el.textContent = factor === 1 ? el.dataset.orig : fmt(parseFloat(el.dataset.q) * factor, el.dataset.s);
      el.classList.toggle("scaled", factor !== 1);
    }
    label.textContent = base ? `${fmt(servings, "dec")} ${T.servings}` : `${fmt(factor, "frac")}×`;
  }

  for (const el of document.querySelectorAll(".q")) el.dataset.orig = el.textContent;

  scaler.addEventListener("click", (e) => {
    const b = e.target.closest("[data-step]");
    if (!b) return;
    const dir = Number(b.dataset.step);
    if (base) {
      servings = Math.max(1, servings + dir);
      factor = servings / base;
    } else {
      factor = Math.max(0.5, factor + dir * 0.5);
    }
    update();
  });
  label.addEventListener("click", () => { servings = base; factor = 1; update(); });
})();

// ---------- Recipe: cook mode + tick-off ----------
(function () {
  const btn = document.getElementById("cook");
  if (!btn) return;
  let lock = null;

  async function keepAwake() {
    try {
      if ("wakeLock" in navigator) lock = await navigator.wakeLock.request("screen");
    } catch (_) { /* needs https or localhost; ignore */ }
  }

  function markCurrent() {
    const steps = [...document.querySelectorAll(".steps li")];
    steps.forEach((li) => li.classList.remove("current"));
    const next = steps.find((li) => !li.classList.contains("done"));
    if (next && document.body.classList.contains("cook")) {
      next.classList.add("current");
    }
    return next;
  }

  btn.addEventListener("click", async () => {
    const on = document.body.classList.toggle("cook");
    btn.textContent = on ? T.exitCookMode : T.cookMode;
    btn.classList.toggle("on", on);
    if (on) await keepAwake();
    else if (lock) { lock.release(); lock = null; }
    markCurrent();
  });

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible" && document.body.classList.contains("cook")) keepAwake();
  });

  document.addEventListener("click", (e) => {
    const li = e.target.closest(".ingredients li, .steps li");
    if (!li) return;
    li.classList.toggle("done");
    if (li.closest(".steps")) {
      const next = markCurrent();
      if (next && document.body.classList.contains("cook")) next.scrollIntoView({ behavior: "smooth", block: "center" });
    }
  });
})();
