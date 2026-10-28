// Program widgets: every .synod-program element becomes an editable, runnable
// program. See the contract in the site build plan (section 3.5):
//
//   <meta name="synod" content='{"assets":"…/assets/","wasm":"…","wasmExec":"…","player":"…","errors":"…/errors/"}'>
//   <div class="synod-program" data-program="raft" data-op="run|check"
//        data-files="#f1" | data-files-url="assets/examples-v0.2.1.json" [data-files-prefix=""]
//        data-show="raft/raft.synod raft/demo.synod" data-predict="on|off">
//     <script type="application/json" id="f1">{"path":"text",…}</script>
//     <pre class="synod" data-file="raft/raft.synod" data-changed="3,4"><code>…</code></pre>
//   </div>
//
// One Web Worker per page runs synod.wasm, created on the first action; the
// newest request of a widget wins and stale results are dropped (decision 109).
// The widget gets data-status="running"|"ok"|"error".

import { createEditor } from "./editor.js";
import { attachPredict } from "./predict.js";

const metaEl = document.querySelector('meta[name="synod"]');
const meta = metaEl ? JSON.parse(metaEl.content) : {};
const assetsBase = new URL(meta.assets || "./", document.baseURI);
const assetUrl = (name) => new URL(name, assetsBase).href;
const errorsBase = new URL(meta.errors || "errors/", document.baseURI);

// ---------------------------------------------------------------- worker

let worker = null;
let initPromise = null;
let nextId = 1;
const waiting = new Map();
const progressListeners = new Set();

function ensureWorker() {
  if (initPromise) return initPromise;
  worker = new Worker(new URL("./worker.js", import.meta.url));
  initPromise = new Promise((resolve, reject) => {
    waiting.set("init", { resolve, reject });
  });
  worker.onmessage = (e) => {
    const m = e.data;
    if (m.progress) {
      for (const f of progressListeners) f(m.progress);
      return;
    }
    const w = waiting.get(m.id);
    if (!w) return;
    waiting.delete(m.id);
    if (m.error !== undefined) w.reject(new Error(m.error));
    else w.resolve(m.result);
  };
  worker.onerror = (e) => {
    const w = waiting.get("init");
    if (w) {
      waiting.delete("init");
      w.reject(new Error(e.message || "worker failed"));
    }
  };
  worker.postMessage({ op: "init", wasmExec: assetUrl(meta.wasmExec), wasm: assetUrl(meta.wasm) });
  initPromise.catch(() => {
    // allow a retry on the next action
    initPromise = null;
    if (worker) worker.terminate();
    worker = null;
  });
  return initPromise;
}

const latest = new Map(); // channel -> newest request id

// Returns the result, or null when a newer request of the channel replaced this one.
async function call(channel, op, files, options) {
  const id = nextId++;
  latest.set(channel, id);
  await ensureWorker();
  const p = new Promise((resolve, reject) => waiting.set(id, { resolve, reject }));
  worker.postMessage({ id, op, files, options });
  const result = await p;
  return latest.get(channel) === id ? result : null;
}

// ---------------------------------------------------------------- player

let playerModule = null;
function loadPlayer() {
  if (!playerModule) playerModule = import(/* @vite-ignore */ assetUrl(meta.player));
  return playerModule;
}

// ---------------------------------------------------------------- helpers

function parseJSON(text) {
  if (typeof text !== "string" || !text.trim()) return null;
  try {
    return JSON.parse(text);
  } catch (_) {
    return null;
  }
}

// diagnostics from a synod-check/1 document, a bare array or a single diagnostic
function diagnosticsOf(text) {
  const v = typeof text === "string" ? parseJSON(text) : text;
  if (!v) return [];
  if (Array.isArray(v)) return v;
  if (Array.isArray(v.diagnostics)) return v.diagnostics;
  if (v.code && v.message) return [v];
  return [];
}

const fileCache = new Map();
async function fetchFiles(url) {
  if (!fileCache.has(url)) {
    fileCache.set(
      url,
      fetch(url).then((r) => {
        if (!r.ok) throw new Error("could not load " + url + " (" + r.status + ")");
        return r.json();
      }),
    );
  }
  return fileCache.get(url);
}

function filesUrl(u) {
  if (/^(https?:)?\/\//.test(u) || u.startsWith("/")) return u;
  // "assets/…" is relative to the version root, the parent of the assets folder
  if (u.startsWith("assets/")) return new URL(u.slice("assets/".length), assetsBase).href;
  return new URL(u, document.baseURI).href;
}

function el(tag, cls, text) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text !== undefined) e.textContent = text;
  return e;
}

function fmtTime(ms) {
  return (ms / 1000).toFixed(1) + "s";
}

// ---------------------------------------------------------------- widget

class Widget {
  constructor(root) {
    this.root = root;
    this.id = "w" + Widget.count++;
    this.program = root.dataset.program || "";
    this.op = root.dataset.op === "check" ? "check" : "run";
    this.predict = root.dataset.predict !== "off";
    this.editors = new Map();
    this.files = {};
    this.original = {};
    this.shown = [];
    this.player = null;
    this.liveTimer = null;
    this.live = 0;
  }

  async init() {
    const root = this.root;
    const pres = new Map();
    for (const p of root.querySelectorAll("pre.synod[data-file]")) pres.set(p.dataset.file, p);
    try {
      this.files = await this.loadFiles();
    } catch (e) {
      this.build();
      this.setStatus("error", e.message);
      return;
    }
    this.original = { ...this.files };
    const show = (root.dataset.show || "").split(/\s+/).filter(Boolean);
    this.shown = show.length ? show : Object.keys(this.files).filter((f) => f.endsWith(".synod")).sort();
    this.pres = pres;
    this.build();
  }

  async loadFiles() {
    const root = this.root;
    const sel = root.dataset.files;
    if (sel) {
      const node = sel.startsWith("#") ? document.getElementById(sel.slice(1)) : root.querySelector(sel);
      const v = node ? parseJSON(node.textContent) : null;
      if (!v) throw new Error("the program's files could not be read");
      return v;
    }
    const url = root.dataset.filesUrl;
    if (!url) throw new Error("the program has no files");
    const all = await fetchFiles(filesUrl(url));
    const prefix = root.dataset.filesPrefix || "";
    if (!prefix) return { ...all };
    // keep the files under the prefix, plus the module file at the root
    const out = {};
    for (const [k, v] of Object.entries(all)) {
      if (k.startsWith(prefix) || k === "synod.mod" || k === "synod.sum") out[k] = v;
    }
    return out;
  }

  build() {
    const root = this.root;
    root.classList.add("synod-program-ready");
    this.tabs = el("div", "sp-tabs");
    this.tabs.setAttribute("role", "tablist");
    this.editorHost = el("div", "sp-editors");
    this.bar = el("div", "sp-bar");
    this.status = el("div", "sp-status");
    this.status.setAttribute("role", "status");
    this.status.setAttribute("aria-live", "polite");
    this.diagList = el("ol", "sp-diags");
    this.diagList.hidden = true;
    this.summary = el("details", "sp-summary");
    this.summary.hidden = true;
    this.summary.append(el("summary", "", "Summary"));
    this.summaryText = el("pre", "sp-summary-text");
    this.summary.append(this.summaryText);
    this.stage = el("div", "sp-stage");
    this.stage.hidden = true;

    for (const path of this.shown) {
      const text = this.files[path] ?? "";
      const ed = createEditor({
        path,
        text,
        pre: this.pres && this.pres.get(path),
        onChange: (t) => this.onEdit(path, t),
      });
      ed.textarea.addEventListener("keydown", (e) => {
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
          e.preventDefault();
          this.primary();
        }
      });
      this.editors.set(path, ed);
      this.editorHost.append(ed.el);
    }
    if (this.pres) for (const p of this.pres.values()) p.remove();

    if (this.shown.length > 1) {
      this.shown.forEach((path, i) => {
        const b = el("button", "sp-tab", path);
        b.type = "button";
        b.setAttribute("role", "tab");
        b.addEventListener("click", () => this.select(path));
        this.tabs.append(b);
        this.tabBtn = this.tabBtn || new Map();
        this.tabBtn.set(path, b);
      });
    }

    this.runBtn = el("button", "synod-btn sp-run", this.op === "check" ? "Check" : "Run");
    this.runBtn.type = "button";
    this.runBtn.addEventListener("click", () => this.primary());
    this.fmtBtn = el("button", "synod-btn synod-btn-quiet sp-format", "Format");
    this.fmtBtn.type = "button";
    this.fmtBtn.addEventListener("click", () => this.format());
    this.resetBtn = el("button", "synod-btn synod-btn-quiet sp-reset", "Reset");
    this.resetBtn.type = "button";
    this.resetBtn.addEventListener("click", () => this.reset());
    this.bar.append(this.runBtn, this.fmtBtn, this.resetBtn);
    if (this.op === "run") {
      const lab = el("label", "sp-seed");
      lab.append(el("span", "", "Seed"));
      this.seed = document.createElement("input");
      this.seed.type = "text";
      this.seed.inputMode = "numeric";
      this.seed.placeholder = "default";
      this.seed.setAttribute("size", "8");
      this.seed.className = "sp-seed-input";
      this.seed.addEventListener("keydown", (e) => {
        if (e.key === "Enter") this.primary();
      });
      lab.append(this.seed);
      this.bar.append(lab);
    }

    // keep any <script> data children; add the UI after them
    root.append(this.tabs, this.editorHost, this.bar, this.status, this.diagList, this.summary, this.stage);
    if (this.shown.length) this.select(this.shown[0]);
  }

  select(path) {
    this.current = path;
    for (const [p, ed] of this.editors) ed.el.hidden = p !== path;
    if (this.tabBtn) {
      for (const [p, b] of this.tabBtn) {
        b.setAttribute("aria-selected", String(p === path));
        b.classList.toggle("active", p === path);
      }
    }
  }

  primary() {
    return this.op === "check" ? this.check() : this.run();
  }

  setStatus(state, text) {
    if (state) this.root.dataset.status = state;
    else delete this.root.dataset.status;
    this.status.textContent = text || "";
    this.status.classList.toggle("is-error", state === "error");
    this.status.classList.toggle("is-ok", state === "ok");
  }

  progressStatus() {
    const f = (p) => {
      const mb = (n) => (n / 1048576).toFixed(1);
      this.setStatus("running", p.total ? "Loading compiler… " + mb(p.loaded) + " of " + mb(p.total) + " MB" : "Loading compiler…");
    };
    progressListeners.add(f);
    return () => progressListeners.delete(f);
  }

  onEdit(path, text) {
    this.files[path] = text;
    clearTimeout(this.liveTimer);
    this.liveTimer = setTimeout(() => this.liveCheck(), 300);
  }

  checkOptions() {
    const o = { tokens: true, style: "short" };
    if (this.program) o.paths = [this.program];
    return o;
  }

  async liveCheck(keepStatus = false) {
    const sent = { ...this.files };
    let res;
    try {
      res = await call(this.id + ":live", "check", sent, this.checkOptions());
    } catch (_) {
      return; // errors surface on the explicit actions
    }
    if (!res || res.error) return;
    this.applyTokens(res, sent);
    // a live check does not replace a running action's result
    if (this.root.dataset.status !== "running") {
      if (!keepStatus) this.setStatus(null, ""); // the last action's result no longer describes the text
      this.showDiagnostics(diagnosticsOf(res.check));
    }
  }

  applyTokens(res, sent) {
    const doc = parseJSON(res.check);
    if (!doc || !Array.isArray(doc.files)) return;
    for (const f of doc.files) {
      const ed = this.editors.get(f.file);
      if (ed) ed.setTokens(f.tokens, sent[f.file] ?? "");
    }
  }

  showDiagnostics(diags) {
    this.root.dataset.diagnostics = String(diags.length);
    this.diagList.replaceChildren();
    this.diagList.hidden = diags.length === 0;
    const byFile = new Map();
    for (const d of diags) {
      if (!byFile.has(d.file)) byFile.set(d.file, []);
      byFile.get(d.file).push(d);
      const li = el("li", "sp-diag sev-" + (d.severity || "error"));
      const where = d.file ? d.file + ":" + d.line + ":" + d.col : "";
      if (where) {
        const a = el("a", "sp-loc", where);
        a.href = "#";
        a.addEventListener("click", (e) => {
          e.preventDefault();
          const ed = this.editors.get(d.file);
          if (ed) {
            this.select(d.file);
            ed.goto(d.line, d.col);
          }
        });
        li.append(a, " ");
      }
      if (d.code) {
        const c = el("a", "sp-code", d.code);
        c.href = new URL(encodeURIComponent(d.code) + "/", errorsBase).href;
        c.target = "_blank";
        c.rel = "noopener";
        li.append(c, " ");
      }
      li.append(el("span", "sp-msg", d.message || ""));
      if (d.hint) li.append(" ", el("span", "sp-hint", d.hint));
      this.diagList.append(li);
    }
    for (const [path, ed] of this.editors) ed.setDiagnostics(byFile.get(path) || []);
  }

  async check() {
    clearTimeout(this.liveTimer);
    this.setStatus("running", "Checking…");
    const stop = this.progressStatus();
    let res;
    try {
      res = await call(this.id + ":act", "check", { ...this.files }, this.checkOptions());
    } catch (e) {
      stop();
      this.setStatus("error", e.message);
      return;
    }
    stop();
    if (!res) return;
    if (res.error) return this.setStatus("error", res.error.message);
    this.applyTokens(res, this.files);
    const diags = diagnosticsOf(res.check);
    this.showDiagnostics(diags);
    if (diags.length) this.setStatus("error", diags.length === 1 ? "1 problem" : diags.length + " problems");
    else this.setStatus("ok", "No problems");
  }

  async run() {
    clearTimeout(this.liveTimer);
    const options = { program: this.program, style: "short", summary: true };
    if (this.seed && this.seed.value.trim() !== "") {
      const s = this.seed.value.trim();
      if (!/^\d+$/.test(s)) return this.setStatus("error", "The seed must be a whole number.");
      options.seed = Number(s);
    }
    this.setStatus("running", "Running…");
    const stop = this.progressStatus();
    let res;
    try {
      res = await call(this.id + ":act", "run", { ...this.files }, options);
    } catch (e) {
      stop();
      this.setStatus("error", e.message);
      return;
    }
    stop();
    if (!res) return;
    if (res.exitCode === 2 || res.error) {
      this.showDiagnostics([]);
      this.summary.hidden = true;
      return this.setStatus("error", res.error ? res.error.message : "The run failed.");
    }
    let diags = diagnosticsOf(res.check);
    let failureNote = "";
    if (res.failure) {
      diags = diags.concat(diagnosticsOf(res.failure.diagnostic));
      failureNote = res.failure.kind ? "Run failed (" + res.failure.kind + ")." : "Run failed.";
    }
    this.showDiagnostics(diags);
    if (res.summary) {
      this.summaryText.textContent = res.summary;
      this.summary.hidden = false;
    } else {
      this.summary.hidden = true;
    }
    const ok = res.exitCode === 0 && !diags.length;
    let text;
    if (diags.length) text = failureNote || (diags.length === 1 ? "1 problem" : diags.length + " problems");
    else text = (res.status || "").replace(/ -> out\/\S+/g, "").trim() || "Done.";
    this.setStatus(ok ? "ok" : "error", text);
    if (res.scene) await this.play(res.scene);
  }

  async play(sceneText) {
    let mod;
    try {
      mod = await loadPlayer();
    } catch (e) {
      this.status.textContent += " (the player could not be loaded: " + e.message + ")";
      return;
    }
    const scene = parseJSON(sceneText);
    if (!this.player) {
      this.buildStage();
      this.stage.hidden = false;
      this.setAspect(scene);
      this.player = mod.createPlayer(this.canvas, sceneText, { autoplay: true, stopAtPause: this.predict });
      this.wirePlayer();
    } else {
      this.stage.hidden = false;
      this.setAspect(scene);
      this.player.setScene(sceneText);
      this.player.seek(0);
      this.player.play();
    }
    this.caption.textContent = "";
    this.slider.max = String(Math.round(this.player.duration));
  }

  setAspect(scene) {
    if (scene && scene.canvas) this.canvas.style.aspectRatio = scene.canvas.w + " / " + scene.canvas.h;
  }

  buildStage() {
    this.canvas = document.createElement("canvas");
    this.canvas.className = "sp-canvas";
    this.canvas.setAttribute("aria-label", "Animation of the run");
    const controls = el("div", "sp-controls");
    this.playBtn = el("button", "synod-btn synod-btn-quiet sp-play", "Pause");
    this.playBtn.type = "button";
    this.slider = document.createElement("input");
    this.slider.type = "range";
    this.slider.min = "0";
    this.slider.value = "0";
    this.slider.className = "sp-slider";
    this.slider.setAttribute("aria-label", "Time");
    this.clock = el("span", "sp-clock", "0.0s");
    controls.append(this.playBtn, this.slider, this.clock);
    this.caption = el("p", "sp-caption");
    this.caption.setAttribute("aria-live", "polite");
    const predictHost = el("div");
    this.predictHost = predictHost;
    this.stage.append(this.canvas, controls, this.caption, predictHost);
  }

  wirePlayer() {
    const p = this.player;
    this.predictUi = attachPredict(p, this.predictHost);
    this.playBtn.addEventListener("click", () => {
      if (!p.playing && p.time >= p.duration - 1) p.seek(0);
      p.toggle();
    });
    this.slider.addEventListener("input", () => {
      p.seek(Number(this.slider.value));
      this.predictUi.hide();
    });
    p.on("time", ({ at }) => {
      this.slider.value = String(Math.round(at));
      this.clock.textContent = fmtTime(at) + " / " + fmtTime(p.duration);
    });
    p.on("play", () => {
      this.playBtn.textContent = "Pause";
    });
    p.on("pause", ({ reason }) => {
      this.playBtn.textContent = reason === "end" ? "Replay" : "Play";
    });
    p.on("scene", () => {
      this.slider.max = String(Math.round(p.duration));
    });
    p.on("marker", ({ marker }) => {
      if (marker.kind !== "pause") {
        this.caption.textContent = marker.text;
        this.caption.dataset.kind = marker.kind;
      }
    });
  }

  async format() {
    clearTimeout(this.liveTimer);
    this.setStatus("running", "Formatting…");
    const stop = this.progressStatus();
    const paths = Object.keys(this.files).filter((f) => f.endsWith(".synod") && this.shown.includes(f));
    let res;
    try {
      res = await call(this.id + ":act", "format", { ...this.files }, { style: "short", paths });
    } catch (e) {
      stop();
      this.setStatus("error", e.message);
      return;
    }
    stop();
    if (!res) return;
    if (res.error) return this.setStatus("error", res.error.message);
    const changed = res.changed || [];
    for (const path of changed) {
      const text = res.files[path];
      if (typeof text === "string") {
        this.files[path] = text;
        const ed = this.editors.get(path);
        if (ed) ed.setText(text);
      }
    }
    const diags = diagnosticsOf(res.check);
    this.showDiagnostics(diags);
    if (diags.length) this.setStatus("error", "Could not format: " + (diags.length === 1 ? "1 problem" : diags.length + " problems"));
    else this.setStatus("ok", changed.length ? "Formatted." : "Already formatted.");
    this.liveCheck(true);
  }

  reset() {
    clearTimeout(this.liveTimer);
    this.files = { ...this.original };
    for (const [path, ed] of this.editors) ed.setText(this.files[path] ?? "");
    this.showDiagnostics([]);
    this.summary.hidden = true;
    this.setStatus(null, "");
    if (worker) this.liveCheck();
  }
}
Widget.count = 0;

function start() {
  for (const root of document.querySelectorAll(".synod-program")) {
    new Widget(root).init();
  }
}

if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start);
else start();
