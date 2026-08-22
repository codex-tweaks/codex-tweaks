// `root` 由 Codex Tweaks 提供。
// 组件放在 Shadow DOM 中，避免与 Codex 自身样式互相污染。

const style = document.createElement("style");
style.textContent = `
  :host {
    all: initial;
    position: fixed;
    inset: 0;
    z-index: 2147483647;
    pointer-events: none;
    font-family: -apple-system, BlinkMacSystemFont, "SF Pro Text", sans-serif;
  }

  .launcher,
  .panel {
    pointer-events: auto;
    color: rgba(255, 255, 255, 0.94);
    background: rgba(32, 32, 36, 0.96);
    box-shadow: 0 10px 28px rgba(0, 0, 0, 0.28);
  }

  .launcher {
    position: fixed;
    right: 18px;
    bottom: 18px;
    width: 40px;
    height: 40px;
    padding: 0;
    border: 0;
    border-radius: 12px;
    font: 700 12px/1 -apple-system, BlinkMacSystemFont, sans-serif;
    cursor: pointer;
  }

  .launcher:hover {
    background: rgba(52, 51, 60, 0.98);
  }

  .launcher:active {
    transform: scale(0.96);
  }

  .launcher:focus-visible {
    outline: 2px solid #9f94ff;
    outline-offset: 2px;
  }

  .panel {
    position: fixed;
    right: 18px;
    bottom: 66px;
    padding: 11px 13px;
    border-radius: 12px;
    font-size: 13px;
    font-weight: 550;
    line-height: 1.35;
    white-space: nowrap;
  }

  @media (prefers-color-scheme: light) {
    .launcher,
    .panel {
      color: rgba(20, 20, 24, 0.94);
      background: rgba(250, 250, 252, 0.98);
      box-shadow: 0 10px 28px rgba(0, 0, 0, 0.16);
    }

    .launcher:hover {
      background: rgba(238, 238, 242, 0.98);
    }
  }

  @media (prefers-reduced-motion: no-preference) {
    .panel {
      animation: codex-tweaks-pop 140ms cubic-bezier(.2, .8, .2, 1);
      transform-origin: bottom right;
    }

    @keyframes codex-tweaks-pop {
      from { opacity: 0; transform: translateY(4px) scale(.98); }
      to { opacity: 1; transform: translateY(0) scale(1); }
    }
  }
`;

const launcher = document.createElement("button");
launcher.className = "launcher";
launcher.type = "button";
launcher.textContent = "CT";
launcher.title = "查看 Codex Tweaks 加载状态";
launcher.setAttribute("aria-label", "查看 Codex Tweaks 加载状态");
launcher.setAttribute("aria-expanded", "false");
launcher.setAttribute("aria-controls", "codex-tweaks-status");

const panel = document.createElement("div");
panel.id = "codex-tweaks-status";
panel.className = "panel";
panel.hidden = true;
panel.setAttribute("role", "status");
panel.setAttribute("aria-live", "polite");
panel.textContent = "CSS 与 JS 加载完成";

launcher.addEventListener("click", () => {
  const isOpen = panel.hidden;
  panel.hidden = !isOpen;
  launcher.setAttribute("aria-expanded", String(isOpen));
});

root.append(style, panel, launcher);
