import "./style.css";

export function activate({ root }) {
  root.dataset.codexTweaksCtSample = "";

  const status = document.createElement("div");
  status.className = "ct-sample-status";
  status.textContent = "codex_tweaks 已注入";
  status.setAttribute("aria-hidden", "true");
  root.append(status);
}
