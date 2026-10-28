// A textarea laid over a highlighted <pre>, kept in sync.
//
// Highlighting comes only from synod-check/1 tokens (decision 108): the page
// is rendered with them at build time (spans class="t-<role>"), and while
// editing, program.js asks synod.wasm check with tokens and calls setTokens.
// There is no lexer here. Until fresh tokens arrive, a line whose text did not
// change keeps the tokens it had; a changed line is shown unhighlighted.
//
// Indentation: Synod is exactly 2 spaces per level and a tab is an error
// (dsl/spec/01-lexical.md), so Tab inserts spaces and never a tab character.

const INDENT = "  ";

function esc(s) {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// Reads tokens back from a server-rendered <pre><code> of t-<role> spans.
function tokensFromDom(code) {
  const out = [];
  let line = 1;
  let col = 1;
  const walk = (node, role) => {
    if (node.nodeType === Node.TEXT_NODE) {
      const parts = node.nodeValue.split("\n");
      parts.forEach((part, i) => {
        if (i > 0) {
          line++;
          col = 1;
        }
        const len = Array.from(part).length;
        if (role && len > 0) out.push({ line, col, len, role });
        col += len;
      });
      return;
    }
    let r = role;
    if (node.classList) {
      for (const c of node.classList) if (c.startsWith("t-")) r = c.slice(2);
    }
    for (const ch of node.childNodes) walk(ch, r);
  };
  walk(code, null);
  return out;
}

function groupTokens(tokens) {
  const by = new Map();
  for (const t of tokens) {
    if (!by.has(t.line)) by.set(t.line, []);
    by.get(t.line).push(t);
  }
  return by;
}

/**
 * @param {object} o
 * @param {string} o.path      file path (for labels)
 * @param {string} o.text      initial text
 * @param {HTMLElement} [o.pre]  server-rendered <pre class="synod"> to take over
 * @param {(text:string)=>void} o.onChange
 */
export function createEditor(o) {
  const root = document.createElement("div");
  root.className = "synod-editor";
  const pre = document.createElement("pre");
  pre.className = "synod synod-hl";
  pre.setAttribute("aria-hidden", "true");
  const code = document.createElement("code");
  pre.append(code);
  const ta = document.createElement("textarea");
  ta.spellcheck = false;
  ta.setAttribute("autocapitalize", "off");
  ta.setAttribute("autocomplete", "off");
  ta.setAttribute("autocorrect", "off");
  ta.setAttribute("wrap", "off");
  ta.setAttribute("aria-label", "Synod source of " + o.path);
  ta.value = o.text;
  root.append(pre, ta);

  let changed = new Set();
  if (o.pre) {
    const ch = o.pre.getAttribute("data-changed");
    if (ch) for (const n of ch.split(",")) changed.add(Number(n));
  }
  let tokenText = o.text;
  let tokenLines = groupTokens(o.pre ? tokensFromDom(o.pre.querySelector("code") || o.pre) : []);
  let diags = [];
  let escapeSeen = false;

  function render() {
    const text = ta.value;
    const lines = text.split("\n");
    const oldLines = tokenText.split("\n");
    // squiggles per line: array of [start,end) in code points
    const sq = new Map();
    for (const d of diags) {
      const endLine = d.end && d.end.line ? d.end.line : d.line;
      const endCol = d.end && d.end.col ? d.end.col : d.col;
      for (let l = d.line; l <= endLine && l <= lines.length; l++) {
        const len = Array.from(lines[l - 1]).length;
        let s = l === d.line ? d.col - 1 : 0;
        let e = l === endLine ? endCol - 1 : len;
        if (e <= s) e = s + 1;
        if (!sq.has(l)) sq.set(l, []);
        sq.get(l).push([s, e, d]);
      }
    }
    const html = [];
    lines.forEach((line, idx) => {
      const n = idx + 1;
      const cps = Array.from(line);
      const roles = new Array(cps.length).fill("");
      const toks = oldLines[idx] === line ? tokenLines.get(n) : null;
      if (toks) {
        for (const t of toks) {
          for (let k = t.col - 1; k < t.col - 1 + t.len && k < cps.length; k++) roles[k] = t.role;
        }
      }
      const marks = sq.get(n) || [];
      const bad = new Array(cps.length + 1).fill(false);
      for (const [s, e] of marks) for (let k = s; k < e && k <= cps.length; k++) bad[k] = true;
      if (bad[cps.length]) cps.push(" "), roles.push("");
      let h = "";
      let i = 0;
      while (i < cps.length) {
        let j = i + 1;
        while (j < cps.length && roles[j] === roles[i] && bad[j] === bad[i]) j++;
        const txt = esc(cps.slice(i, j).join(""));
        const cls = [];
        if (roles[i]) cls.push("t-" + roles[i]);
        if (bad[i]) cls.push("squiggle");
        h += cls.length ? '<span class="' + cls.join(" ") + '">' + txt + "</span>" : txt;
        i = j;
      }
      const cl = changed.has(n) ? ' class="changed"' : "";
      html.push(changed.has(n) ? "<span" + cl + ">" + h + "</span>" : h);
    });
    code.innerHTML = html.join("\n") + "​";
  }

  function syncScroll() {
    pre.scrollLeft = ta.scrollLeft;
  }

  // Replace [start,end) of the text, keeping the undo stack where the browser allows.
  function edit(start, end, repl, selStart, selEnd) {
    ta.focus();
    ta.setSelectionRange(start, end);
    let done = false;
    try {
      done = document.execCommand("insertText", false, repl);
    } catch (_) {
      done = false;
    }
    if (!done) {
      ta.setRangeText(repl, start, end, "end");
      ta.dispatchEvent(new Event("input", { bubbles: true }));
    }
    ta.setSelectionRange(selStart, selEnd);
  }

  function lineStart(pos) {
    return ta.value.lastIndexOf("\n", pos - 1) + 1;
  }

  function onKey(e) {
    if (e.key === "Escape") {
      escapeSeen = true; // next Tab leaves the editor
      return;
    }
    const v = ta.value;
    const s = ta.selectionStart;
    const en = ta.selectionEnd;
    if (e.key === "Tab" && !e.ctrlKey && !e.metaKey && !e.altKey) {
      if (escapeSeen) {
        escapeSeen = false;
        return;
      }
      e.preventDefault();
      const multi = v.slice(s, en).includes("\n");
      if (!e.shiftKey && !multi) {
        // to the next multiple of 2 columns
        const col = s - lineStart(s);
        const n = INDENT.length - (col % INDENT.length);
        edit(s, en, " ".repeat(n), s + n, s + n);
        return;
      }
      // indent or dedent every line touched
      const a = lineStart(s);
      let b = en;
      if (en > s && v[en - 1] === "\n") b = en - 1;
      const nl = v.indexOf("\n", b);
      const bEnd = nl < 0 ? v.length : nl;
      const block = v.slice(a, bEnd).split("\n");
      let firstDelta = 0;
      let total = 0;
      const out = block.map((ln, i) => {
        let r;
        let delta;
        if (e.shiftKey) {
          const m = ln.match(/^ {1,2}/);
          const k = m ? m[0].length : 0;
          r = ln.slice(k);
          delta = -k;
        } else {
          r = ln.length ? INDENT + ln : ln;
          delta = ln.length ? INDENT.length : 0;
        }
        if (i === 0) firstDelta = delta;
        total += delta;
        return r;
      });
      edit(a, bEnd, out.join("\n"), Math.max(a, s + firstDelta), en + total);
      return;
    }
    if (e.key === "Enter" && !e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey) {
      const a = lineStart(s);
      const m = v.slice(a, s).match(/^ */);
      const ind = m[0];
      e.preventDefault();
      edit(s, en, "\n" + ind, s + 1 + ind.length, s + 1 + ind.length);
      return;
    }
    if (e.key === "Backspace" && s === en && s > 0 && !e.ctrlKey && !e.metaKey && !e.altKey) {
      const a = lineStart(s);
      const before = v.slice(a, s);
      if (before.length > 0 && /^ +$/.test(before)) {
        const n = before.length % INDENT.length || INDENT.length;
        e.preventDefault();
        edit(s - n, s, "", s - n, s - n);
      }
    }
  }

  ta.addEventListener("keydown", onKey);
  ta.addEventListener("blur", () => {
    escapeSeen = false;
  });
  ta.addEventListener("scroll", syncScroll);
  ta.addEventListener("input", () => {
    changed = new Set();
    render();
    o.onChange(ta.value);
  });

  render();

  return {
    el: root,
    textarea: ta,
    getText: () => ta.value,
    setText(text, notify = false) {
      ta.value = text;
      changed = new Set();
      render();
      if (notify) o.onChange(text);
    },
    /** tokens: synod-check/1 files[].tokens computed for `forText`. */
    setTokens(tokens, forText) {
      tokenText = forText;
      tokenLines = groupTokens(tokens || []);
      render();
    },
    setDiagnostics(list) {
      diags = list || [];
      render();
    },
    focus() {
      ta.focus();
    },
    goto(line, col) {
      const lines = ta.value.split("\n");
      let pos = 0;
      for (let i = 0; i < line - 1 && i < lines.length; i++) pos += lines[i].length + 1;
      pos += Math.max(0, Math.min((col || 1) - 1, (lines[line - 1] || "").length));
      ta.focus();
      ta.setSelectionRange(pos, pos);
    },
  };
}
