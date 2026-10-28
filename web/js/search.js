// Site search: loads search.json ([{u,t,s,x}]: url, title, section, excerpt)
// once, builds an in-memory prefix index and lists results under the header box.
//
// Markup: <input type="search" data-synod-search data-search-url="…/search.json">
//         <div data-synod-search-results hidden></div>
// Without data-search-url the file is "search.json" next to the assets folder.
// "/" focuses the box; Up/Down/Enter/Escape work in the result list.

const input = document.querySelector("[data-synod-search]");
const box = document.querySelector("[data-synod-search-results]");

if (input && box) {
  let docs = null;
  let loading = null;
  let words = []; // sorted unique words
  let postings = new Map(); // word -> doc ids
  let active = -1;

  const metaEl = document.querySelector('meta[name="synod"]');
  const meta = metaEl ? JSON.parse(metaEl.content) : {};
  const url =
    input.dataset.searchUrl || new URL("../search.json", new URL(meta.assets || "./", document.baseURI)).href;

  const split = (s) => (s.toLowerCase().match(/[\p{L}\p{N}_]+/gu) || []);

  function build(list) {
    docs = list;
    postings = new Map();
    list.forEach((d, i) => {
      const seen = new Set();
      for (const w of split([d.t, d.s, d.x].filter(Boolean).join(" "))) {
        if (seen.has(w)) continue;
        seen.add(w);
        if (!postings.has(w)) postings.set(w, []);
        postings.get(w).push(i);
      }
    });
    words = [...postings.keys()].sort();
  }

  function load() {
    if (!loading) {
      loading = fetch(url)
        .then((r) => {
          if (!r.ok) throw new Error("search index " + r.status);
          return r.json();
        })
        .then(build)
        .catch((e) => {
          loading = null;
          throw e;
        });
    }
    return loading;
  }

  // first index in words with words[i] >= p
  function lower(p) {
    let lo = 0;
    let hi = words.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (words[mid] < p) lo = mid + 1;
      else hi = mid;
    }
    return lo;
  }

  function docsWithPrefix(p) {
    const out = new Set();
    for (let i = lower(p); i < words.length && words[i].startsWith(p); i++) {
      for (const d of postings.get(words[i])) out.add(d);
    }
    return out;
  }

  function search(q) {
    const terms = split(q);
    if (!terms.length) return [];
    let hit = null;
    for (const t of terms) {
      const s = docsWithPrefix(t);
      hit = hit ? new Set([...hit].filter((d) => s.has(d))) : s;
      if (!hit.size) return [];
    }
    const lq = q.toLowerCase().trim();
    const scored = [...hit].map((i) => {
      const d = docs[i];
      const title = (d.t || "").toLowerCase();
      let score = 0;
      if (title === lq) score += 100;
      else if (title.startsWith(lq)) score += 50;
      for (const t of terms) if (split(d.t || "").some((w) => w.startsWith(t))) score += 10;
      return { i, score };
    });
    scored.sort((a, b) => b.score - a.score || a.i - b.i);
    return scored.slice(0, 12).map((x) => docs[x.i]);
  }

  function render(results, q) {
    box.replaceChildren();
    active = -1;
    if (!q.trim()) {
      box.hidden = true;
      return;
    }
    box.hidden = false;
    if (!results.length) {
      const p = document.createElement("p");
      p.className = "search-none";
      p.textContent = "Nothing found for “" + q + "”.";
      box.append(p);
      return;
    }
    const ul = document.createElement("ul");
    ul.className = "search-list";
    for (const d of results) {
      const li = document.createElement("li");
      const a = document.createElement("a");
      a.href = d.u;
      const t = document.createElement("span");
      t.className = "search-title";
      t.textContent = d.t;
      a.append(t);
      if (d.s) {
        const s = document.createElement("span");
        s.className = "search-section";
        s.textContent = d.s;
        a.append(s);
      }
      if (d.x) {
        const x = document.createElement("span");
        x.className = "search-excerpt";
        x.textContent = d.x;
        a.append(x);
      }
      li.append(a);
      ul.append(li);
    }
    box.append(ul);
  }

  function links() {
    return [...box.querySelectorAll("a")];
  }

  function setActive(n) {
    const l = links();
    if (!l.length) return;
    active = (n + l.length) % l.length;
    l[active].focus();
  }

  let timer = null;
  input.addEventListener("input", () => {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      try {
        await load();
      } catch (e) {
        box.hidden = false;
        box.textContent = "Search is unavailable.";
        return;
      }
      render(search(input.value), input.value);
    }, 60);
  });
  input.addEventListener("focus", () => {
    load().catch(() => {});
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive(0);
    } else if (e.key === "Escape") {
      input.value = "";
      render([], "");
    } else if (e.key === "Enter") {
      const a = links()[0];
      if (a) location.href = a.href;
    }
  });
  box.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive(active + 1);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (active <= 0) input.focus();
      else setActive(active - 1);
    } else if (e.key === "Escape") {
      input.focus();
      render([], "");
      input.value = "";
    }
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "/" && !/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)) {
      e.preventDefault();
      input.focus();
    }
  });
  document.addEventListener("click", (e) => {
    if (!box.contains(e.target) && e.target !== input) box.hidden = true;
  });
  input.addEventListener("focus", () => {
    if (box.childElementCount) box.hidden = false;
  });
}
