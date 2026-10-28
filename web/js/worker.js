// Web Worker that runs synod.wasm. Classic worker (importScripts), one per page.
//
// Messages in:  {op:"init", wasmExec, wasm}
//               {id, op:"check"|"run"|"format"|"version", files, options}
// Messages out: {progress:{loaded,total}}   while the wasm downloads
//               {ready:true, version}        after init
//               {id, result} | {id, error}   per request (init uses id "init")
"use strict";

let ready = null;

function post(msg) {
  self.postMessage(msg);
}

// fetch the wasm, reporting progress; streaming compile with an arrayBuffer fallback
async function instantiate(url, imports) {
  const resp = await fetch(url);
  if (!resp.ok) throw new Error("could not load " + url + " (" + resp.status + ")");
  const total = Number(resp.headers.get("Content-Length")) || 0;
  let loaded = 0;
  let body = resp;
  if (resp.body && typeof ReadableStream === "function") {
    const reader = resp.body.getReader();
    const stream = new ReadableStream({
      async pull(ctl) {
        const { done, value } = await reader.read();
        if (done) {
          ctl.close();
          return;
        }
        loaded += value.byteLength;
        post({ progress: { loaded, total: Math.max(total, loaded) } });
        ctl.enqueue(value);
      },
      cancel(reason) {
        return reader.cancel(reason);
      },
    });
    body = new Response(stream, { headers: { "Content-Type": "application/wasm" } });
  }
  if (WebAssembly.instantiateStreaming) {
    try {
      return await WebAssembly.instantiateStreaming(body, imports);
    } catch (e) {
      if (body !== resp) {
        // the stream is spent; refetch below
      }
    }
  }
  const again = await fetch(url);
  const buf = await again.arrayBuffer();
  post({ progress: { loaded: buf.byteLength, total: buf.byteLength } });
  return WebAssembly.instantiate(buf, imports);
}

async function init(msg) {
  importScripts(msg.wasmExec);
  const go = new Go();
  const { instance } = await instantiate(msg.wasm, go.importObject);
  go.run(instance); // never resolves: the program waits
  for (let i = 0; i < 500 && !globalThis.synod; i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  if (!globalThis.synod) throw new Error("synod.wasm did not start");
  const v = globalThis.synod.version();
  if (!v || v.api !== 1) throw new Error("synod.wasm API version " + (v && v.api) + " is not supported (need 1)");
  return v;
}

self.onmessage = async (e) => {
  const msg = e.data;
  if (msg.op === "init") {
    if (!ready) ready = init(msg);
    try {
      const v = await ready;
      post({ id: "init", result: v });
    } catch (err) {
      post({ id: "init", error: String((err && err.message) || err) });
    }
    return;
  }
  try {
    if (!ready) throw new Error("worker not initialised");
    await ready;
    const fn = globalThis.synod[msg.op];
    if (typeof fn !== "function") throw new Error("unknown operation " + msg.op);
    const result = msg.op === "version" ? fn() : fn(msg.files || {}, msg.options || {});
    post({ id: msg.id, result });
  } catch (err) {
    post({ id: msg.id, error: String((err && err.message) || err) });
  }
};
