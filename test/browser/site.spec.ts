// Tests of the built site. The pages to test are read from the site itself, so
// a new chapter or example is tested without touching this file.

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { expect, type Locator, type Page, test } from "@playwright/test";

const siteDir = resolve(import.meta.dirname, process.env.SITE_DIR ?? "../../_site");

function readJSON<T>(file: string): T {
  return JSON.parse(readFileSync(file, "utf8")) as T;
}

const versions = readJSON<{ latest: string }>(join(siteDir, "docs", "versions.json"));
const latest = versions.latest;
const docsDir = join(siteDir, "docs", latest);
const docs = `/docs/${latest}/`;

/** The page folders of one section of the docs, e.g. tutorial/01-hello. */
function sectionPages(section: string): string[] {
  const dir = join(docsDir, section);
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isDirectory() && existsSync(join(dir, e.name, "index.html")))
    .map((e) => `${section}/${e.name}`)
    .sort();
}

/** How many program widgets a built page holds. */
function widgetCount(rel: string): number {
  const html = readFileSync(join(docsDir, rel, "index.html"), "utf8");
  return (html.match(/class="synod-program"/g) ?? []).length;
}

/** Fails the test on uncaught errors of the page. */
function failOnPageErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  return errors;
}

const COMMENT = "# edited by the browser test";

/**
 * Runs one widget, edits its first file, runs it again and checks that the
 * edit is in the editor, was accepted, and the program still ran.
 */
async function runEditRerun(widget: Locator): Promise<void> {
  await widget.scrollIntoViewIfNeeded();
  const run = widget.locator(".sp-run");
  await expect(run).toBeVisible();
  await run.click();
  await expect(widget).toHaveAttribute("data-status", "ok");

  // Edit: a comment above the first declaration, which every program may have.
  const area = widget.locator("textarea").first();
  const before = await area.inputValue();
  const lines = before.split("\n");
  let at = lines.findIndex((l, i) => i > 0 && l.trim() !== "" && !l.startsWith("#") && !l.startsWith("use "));
  if (at < 0) at = lines.length;
  lines.splice(at, 0, COMMENT);
  const after = lines.join("\n");
  await area.fill(after);
  await expect(area).toHaveValue(after);
  await expect(widget.locator("pre.synod-hl").first()).toContainText(COMMENT);

  // The live check settles (it clears the status of the previous run), then
  // the edited text runs again.
  await expect(widget).not.toHaveAttribute("data-status", /.+/);
  await run.click();
  await expect(widget).toHaveAttribute("data-status", "ok");
  await expect(area).toHaveValue(after);
  await expect(widget).toHaveAttribute("data-diagnostics", "0");
}

for (const section of ["tutorial", "examples"]) {
  for (const rel of sectionPages(section)) {
    const n = widgetCount(rel);
    test(`${rel}: ${n} widget(s) run, edit and re-run`, async ({ page }) => {
      expect(n, `${rel} has no program widget`).toBeGreaterThan(0);
      const errors = failOnPageErrors(page);
      await page.goto(`${docs}${rel}/`);
      const widgets = page.locator(".synod-program");
      await expect(widgets).toHaveCount(n);
      for (let i = 0; i < n; i++) {
        const w = widgets.nth(i);
        await expect(w).toHaveClass(/synod-program-ready/);
        await runEditRerun(w);
      }
      expect(errors).toEqual([]);
    });
  }
}

test("an Errors page shows its diagnostic", async ({ page }) => {
  const pages = sectionPages("errors");
  expect(pages.length, "the site has no Errors pages").toBeGreaterThan(0);
  const rel = pages.includes("errors/unknown-phrase") ? "errors/unknown-phrase" : (pages[0] as string);
  const code = rel.split("/")[1] as string;
  const errors = failOnPageErrors(page);
  await page.goto(`${docs}${rel}/`);
  const widget = page.locator(".synod-program").first();
  await expect(widget).toHaveClass(/synod-program-ready/);
  await widget.locator(".sp-run").click();
  // The example fails with exactly this code; the diagnostic list names it.
  await expect(widget).toHaveAttribute("data-status", "error");
  await expect(widget.locator(".sp-diags .sp-code").first()).toHaveText(code);
  expect(errors).toEqual([]);
});

test("search finds a page by its title", async ({ page }) => {
  const index = readJSON<{ u: string; t: string; s: string }[]>(join(docsDir, "search.json"));
  expect(index.length).toBeGreaterThan(0);
  const entry = index.find((e) => e.s === "Errors" && !e.u.includes("#")) ?? (index[0] as (typeof index)[number]);
  await page.goto(docs);
  const box = page.locator('input[type="search"]').first();
  await box.fill(entry.t);
  const first = page.locator('#search-results a, [data-synod-search-results] a').first();
  await expect(first).toBeVisible();
  await expect(first).toHaveAttribute("href", new RegExp(`${entry.u.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`));
});

test("/docs/ leads to the latest docs", async ({ page }) => {
  await page.goto("/docs/");
  await page.waitForURL(`**${docs}`);
  expect(new URL(page.url()).pathname).toBe(docs);
});

test("the Looks & Themes page: every package demo runs, edits and re-runs", async ({ page }) => {
  const n = widgetCount("looks");
  expect(n, "the looks page has no demo widget").toBeGreaterThanOrEqual(7);
  const errors = failOnPageErrors(page);
  await page.goto(`${docs}looks/`);
  const widgets = page.locator(".synod-program");
  await expect(widgets).toHaveCount(n);
  for (let i = 0; i < n; i++) {
    const w = widgets.nth(i);
    await expect(w).toHaveClass(/synod-program-ready/);
    await runEditRerun(w);
    // the run is drawn: the stage is shown with a canvas
    await expect(w.locator("canvas.sp-canvas")).toBeVisible();
  }
  expect(errors).toEqual([]);
});

test("the gallery plays every look x theme cell", async ({ page }) => {
  const html = readFileSync(join(docsDir, "looks", "gallery", "index.html"), "utf8");
  const cells = (html.match(/class="gal-cell"/g) ?? []).length;
  expect(cells).toBe(18);
  const errors = failOnPageErrors(page);
  await page.goto(`${docs}looks/gallery/`);
  const all = page.locator(".gal-cell");
  await expect(all).toHaveCount(cells);
  for (let i = 0; i < cells; i++) {
    const cell = all.nth(i);
    await cell.scrollIntoViewIfNeeded();
    await expect(cell).toHaveAttribute("data-state", /^(playing|still)$/);
  }
  // a playing cell has drawn something: its canvas is not a single color
  const first = all.nth(0);
  await first.scrollIntoViewIfNeeded();
  await page.waitForTimeout(500);
  const varied = await first.locator("canvas").evaluate((c: HTMLCanvasElement) => {
    const d = c.getContext("2d")?.getImageData(0, 0, c.width, c.height).data;
    if (!d) return false;
    const seen = new Set<number>();
    for (let i = 0; i < d.length && seen.size < 4; i += 4 * 997) seen.add((d[i] << 16) | (d[i + 1] << 8) | d[i + 2]);
    return seen.size > 1;
  });
  expect(varied).toBe(true);
  // the toggle pauses a cell
  await first.locator(".gal-toggle").click();
  await expect(first).toHaveAttribute("data-state", "paused");
  expect(errors).toEqual([]);
});
