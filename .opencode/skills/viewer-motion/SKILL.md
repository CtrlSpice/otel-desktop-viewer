---
name: Viewer Motion
description: Refine animation, transitions, tactile dragging, or visual polish in the telemetry viewer. Use for motion design and consistency across its dense product UI.
---

# Viewer motion

The desired feel is smooth, quiet, and tactile: objects follow the user's hand,
then settle into place. Motion should be barely noticeable during repeated work.

## Read first

- Read the applicable `AGENTS.md` and the existing component before editing.
- Read `references/emil/animate.md` for motion decisions and interruption.
- Read `references/impeccable/operate.md` for dense product UI, or
  `references/impeccable/polish.md` for a polish pass.
- `references/SOURCES.md` records upstream revisions, licenses, and further reading.
  These are reference documents. Apply this project's requirements when an
  upstream recommendation conflicts with the existing design.

## App-specific rules

- Preserve Rosé Pine Dawn, Moon, and Pine, semantic DaisyUI colours, typography,
  density, pane geometry, focus, selection, routes, and stored sizes.
- Reuse the motion tokens in `desktopexporter/internal/frontend/src/app.css`.
  Feedback is 120ms; opacity fades are 160ms and linear; compact layout changes
  are 180ms; panel settling is 240ms. Layout changes share the settle curve.
- During dragging, track pointer displacement directly. Disable layout transitions
  for every participating panel and divider. Animate the release, not the drag.
- Support reversal from the currently rendered position. Do not restart from a
  hard-coded endpoint or queue interactions behind an animation.
- Keep frequent selection and keyboard actions immediate. Preserve interruptible
  scrolling; wheel, pointer, touch, and keyboard input take precedence.
- Fade the `.pointy-uppy` control without translating or scaling it. Its name is a
  CSS class; its accessible label and tooltip describe scrolling to the top.
- Keep hidden controls out of keyboard and pointer interaction. Retain a focused
  scroll control until focus leaves it, even when scrolling reaches the top.
- Respect reduced motion. Do not add bounce, elastic overshoot, parallax, hover
  lifts, or `transition: all` to the viewer.
- Prefer opacity and transforms for decoration. Layout animation is appropriate
  when adjacent panels really need to exchange space; verify it on a large trace.

## Verify the feel

Use the running app: scroll both ways across a visibility threshold, reverse a
panel toggle mid-flight, drag and reverse before release, and use the keyboard.
Inspect Dawn and a dark theme, and check reduced motion. Check geometry and
focus as well as appearance. Run the relevant existing checks and rebuild
embedded assets as required by the frontend instructions.
