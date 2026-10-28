// The look x theme gallery: every .gal-cell plays its prebuilt scene (made by
// the native `synod run` at build time) with the player. A cell loads its
// scene when it first scrolls into view, plays while it is visible and pauses
// when it leaves; with reduced motion it shows a still frame. The cell's
// data-state is idle, loading, playing, paused, still or error.

const metaEl = document.querySelector('meta[name="synod"]');
const meta = metaEl ? JSON.parse(metaEl.content) : {};
const assetsBase = new URL(meta.assets || "./", document.baseURI);
const url = (name) => new URL(name, assetsBase).href;
const reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

let playerModule = null;
const loadPlayer = () => (playerModule ||= import(/* @vite-ignore */ url(meta.player)));

class Cell {
  constructor(root) {
    this.root = root;
    this.canvas = root.querySelector("canvas");
    this.button = root.querySelector(".gal-toggle");
    this.player = null;
    this.visible = false;
    this.userPaused = false;
    this.button.addEventListener("click", () => {
      this.userPaused = !this.userPaused;
      this.sync();
    });
  }

  async start() {
    if (this.player || this.loading) return;
    this.loading = true;
    this.root.dataset.state = "loading";
    try {
      const [mod, text] = await Promise.all([
        loadPlayer(),
        fetch(url(this.root.dataset.scene.replace(/^assets\//, ""))).then((r) => {
          if (!r.ok) throw new Error("scene " + r.status);
          return r.text();
        }),
      ]);
      const opts = { autoplay: false, loop: true };
      if (meta.fontBase) opts.fontBase = new URL(meta.fontBase, document.baseURI).href;
      this.player = mod.createPlayer(this.canvas, text, opts);
      if (this.player.fontsReady) await this.player.fontsReady(); // first frame after the fonts
      const scene = JSON.parse(text);
      if (scene.canvas) this.canvas.style.aspectRatio = scene.canvas.w + " / " + scene.canvas.h;
      if (reduce) {
        this.player.renderFrame(this.player.duration * 0.35);
        this.userPaused = true;
        this.root.dataset.state = "still";
        this.button.textContent = "Play";
      } else {
        this.sync();
      }
    } catch (e) {
      this.root.dataset.state = "error";
      this.root.querySelector("figcaption span").append(" (could not play: " + e.message + ")");
    }
    this.loading = false;
  }

  sync() {
    if (!this.player) return;
    const run = this.visible && !this.userPaused;
    if (run) this.player.play();
    else this.player.pause();
    this.root.dataset.state = run ? "playing" : "paused";
    this.button.textContent = this.userPaused ? "Play" : "Pause";
  }
}

const cells = new Map();
const io = new IntersectionObserver(
  (entries) => {
    for (const e of entries) {
      const cell = cells.get(e.target);
      cell.visible = e.isIntersecting;
      if (e.isIntersecting && !cell.player) cell.start();
      else cell.sync();
    }
  },
  { rootMargin: "100px" },
);
for (const el of document.querySelectorAll(".gal-cell")) {
  cells.set(el, new Cell(el));
  io.observe(el);
}
