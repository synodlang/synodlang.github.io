// Predict on the site (decision 109): when playback reaches a pause marker the
// question and its choices are shown, the reader may pick one, and Continue
// resumes. Nothing is graded and nothing is stored: a pick only marks the button.
//
// attachPredict(player, host) returns {destroy}. host is an empty element the
// panel is rendered in; it is hidden unless a pause marker is active.

export function attachPredict(player, host) {
  host.hidden = true;
  host.className = "synod-predict";
  let off = [];

  function hide() {
    host.hidden = true;
    host.replaceChildren();
  }

  function show(marker) {
    host.replaceChildren();
    const q = document.createElement("p");
    q.className = "synod-predict-q";
    q.textContent = marker.text;
    host.append(q);
    const choices = marker.choices || [];
    if (choices.length) {
      const ul = document.createElement("div");
      ul.className = "synod-predict-choices";
      ul.setAttribute("role", "group");
      ul.setAttribute("aria-label", "Your prediction");
      for (const c of choices) {
        const b = document.createElement("button");
        b.type = "button";
        b.className = "synod-choice";
        b.textContent = c;
        b.setAttribute("aria-pressed", "false");
        b.addEventListener("click", () => {
          for (const x of ul.children) x.setAttribute("aria-pressed", String(x === b));
        });
        ul.append(b);
      }
      host.append(ul);
    }
    const go = document.createElement("button");
    go.type = "button";
    go.className = "synod-btn synod-continue";
    go.textContent = "Continue";
    go.addEventListener("click", () => {
      hide();
      player.play();
    });
    host.append(go);
    host.hidden = false;
    go.focus({ preventScroll: true });
  }

  let pending = null;
  off.push(
    player.on("marker", ({ marker }) => {
      if (marker.kind === "pause") pending = marker;
    }),
  );
  off.push(
    player.on("pause", ({ reason }) => {
      if (reason === "marker" && pending) {
        show(pending);
        pending = null;
      }
    }),
  );
  off.push(player.on("play", hide));
  off.push(player.on("scene", hide));

  return {
    hide,
    destroy() {
      for (const f of off) f();
      off = [];
      hide();
    },
  };
}
