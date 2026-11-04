// home-v046.js updates the landing page pointer glow while motion is enabled.
// Fine-pointer devices receive the effect when their motion preference allows it.
// Pointer coordinates remain local to this page and the most recent event supplies
// both axes for the next animation frame, reducing repeated style writes during
// bursts of pointer movement.
//
// One frame handle represents scheduled drawing. Disabling the media query removes
// the pointer listener and cancels its pending frame before resetting the handle.
// The media-query listener stays with this document and can enable the effect
// again if the browser's pointer capabilities or motion preference changes.
(() => {
  const glow = document.querySelector(".cursor-glow");
  if (!glow) return;

  const enabled = window.matchMedia(
    "(hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference)"
  );
  let frame = 0;
  let x = 0;
  let y = 0;

  // draw publishes the latest pointer coordinates and releases frame ownership.
  function draw() {
    frame = 0;
    glow.style.setProperty("--x", `${x}px`);
    glow.style.setProperty("--y", `${y}px`);
  }

  // move records pointer coordinates and schedules at most one drawing callback.
  // Repeated events replace the retained coordinates, so the next frame uses the
  // newest position while earlier events require no additional style writes.
  function move(event) {
    x = event.clientX;
    y = event.clientY;
    // Share one drawing callback across pointer events so layout updates happen with
    // rendering and use the last coordinates observed before that callback executes.
    if (!frame) frame = window.requestAnimationFrame(draw);
  }

  // update applies current media-query eligibility to pointer observation. Disabling
  // removes the listener and cancels pending drawing; enabling registers the same
  // callback, which the browser deduplicates across repeated matching notifications.
  function update() {
    if (enabled.matches) {
      window.addEventListener("pointermove", move, { passive: true });
    } else {
      window.removeEventListener("pointermove", move);
      window.cancelAnimationFrame(frame);
      frame = 0;
    }
  }

  enabled.addEventListener("change", update);
  update();
})();
