import "./style.css";

export function activate({ root }) {
  root.setAttribute("data-codex-tweaks-ct-sample", "");

  const status = document.createElement("div");
  status.className = "ct-sample-status";
  status.textContent = "Codex Tweaks 已注入";
  status.setAttribute("aria-hidden", "true");
  root.append(status);
}
