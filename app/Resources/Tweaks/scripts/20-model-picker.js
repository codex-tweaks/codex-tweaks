const CodexTweaksPretext = api.getLibrary("pretext");

// 把模型按首个斜杠前缀整理成悬浮展开的固定两级菜单。
// 无前缀模型统一放入 Codex 分类。第二层移动并复用 Codex 原生模型项，
// 因而选择、勾选和关闭菜单仍完全由 Codex 自己处理。

const MODEL_MENU_MARKER = "data-codex-tweaks-model-menu";
const MODEL_SUBMENU_MARKER = "data-codex-tweaks-model-submenu";
const DEFAULT_GROUP_KEY = "unprefixed";
const DEFAULT_GROUP_LABEL = "Codex";
const HOVER_OPEN_DELAY = 90;
const HOVER_CLOSE_DELAY = 180;
const SUBMENU_GAP = 2;
const VIEWPORT_MARGIN = 8;
const ROOT_MENU_FALLBACK_WIDTH = 220;
const MODEL_SUBMENU_FALLBACK_WIDTH = 210;
const MIN_ROOT_MENU_WIDTH = 180;
const MIN_MODEL_SUBMENU_WIDTH = 180;
const MODEL_SUBMENU_TEXT_BUFFER = 12;
const modelMenuStates = new Map();
let modelMenuScanQueued = false;

function getDirectMenuItems(menu) {
  return [...menu.querySelectorAll('[role="menuitem"]')].filter(
    (item) => item.closest('[role="menu"]') === menu,
  );
}

function getLabelElement(item, fullName) {
  const leafSpans = [...item.querySelectorAll("span")].filter(
    (span) => span.children.length === 0 && span.textContent.trim(),
  );

  return (
    leafSpans.find((span) => span.textContent.trim() === fullName) ??
    leafSpans[0] ??
    null
  );
}

function getPretextMeasurementOptions(element) {
  const style = getComputedStyle(element);
  const font = [
    style.fontStyle,
    style.fontWeight,
    style.fontSize,
    style.fontFamily,
  ].join(" ");
  const parsedLetterSpacing = Number.parseFloat(style.letterSpacing);

  return {
    font,
    letterSpacing: Number.isFinite(parsedLetterSpacing)
      ? parsedLetterSpacing
      : 0,
  };
}

function measureTextWithPretext(text, element) {
  const { font, letterSpacing } = getPretextMeasurementOptions(element);

  try {
    const prepared = CodexTweaksPretext.prepareWithSegments(text, font, {
      letterSpacing,
      wordBreak: "keep-all",
    });
    return CodexTweaksPretext.measureNaturalWidth(prepared);
  } catch {
    return element.scrollWidth || element.offsetWidth || 0;
  }
}

function parsePixelValue(value) {
  const parsed = Number.parseFloat(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function getElementInlineSize(element) {
  return element.offsetWidth || element.getBoundingClientRect().width || 0;
}

function measureFixedInlineSpace(menu, label) {
  let fixedWidth = 0;
  let current = label;

  while (current && current !== menu) {
    const currentStyle = getComputedStyle(current);
    fixedWidth +=
      parsePixelValue(currentStyle.marginInlineStart) +
      parsePixelValue(currentStyle.marginInlineEnd);

    const parent = current.parentElement;
    if (!parent || !menu.contains(parent)) break;

    const parentStyle = getComputedStyle(parent);
    fixedWidth +=
      parsePixelValue(parentStyle.paddingInlineStart) +
      parsePixelValue(parentStyle.paddingInlineEnd) +
      Math.max(0, parent.offsetWidth - parent.clientWidth);

    const isInlineFlex =
      ["flex", "inline-flex"].includes(parentStyle.display) &&
      parentStyle.flexDirection.startsWith("row");

    if (isInlineFlex) {
      const visibleChildren = [...parent.children].filter(
        (child) => getComputedStyle(child).display !== "none",
      );
      const siblingWidth = visibleChildren
        .filter((child) => child !== current)
        .reduce((total, child) => {
          const childStyle = getComputedStyle(child);
          return (
            total +
            getElementInlineSize(child) +
            parsePixelValue(childStyle.marginInlineStart) +
            parsePixelValue(childStyle.marginInlineEnd)
          );
        }, 0);
      const gaps = Math.max(0, visibleChildren.length - 1);

      fixedWidth +=
        siblingWidth + gaps * parsePixelValue(parentStyle.columnGap);
    }

    current = parent;
  }

  return fixedWidth;
}

function measureMenuWidth(menu, entries, fallbackWidth, minimumWidth) {
  let desiredWidth = minimumWidth;

  for (const { label, text } of entries) {
    const naturalTextWidth = Math.max(
      measureTextWithPretext(text, label),
      label.scrollWidth,
    );
    desiredWidth = Math.max(
      desiredWidth,
      naturalTextWidth + measureFixedInlineSpace(menu, label),
    );
  }

  return Math.ceil(Number.isFinite(desiredWidth) ? desiredWidth : fallbackWidth);
}

function sizeRootModelMenu(state) {
  const entries = [...state.groups.values()].map((group) => ({
    label: state.categories
      .get(group.key)
      .querySelector("[data-codex-tweaks-model-category-label]"),
    text: group.label,
  }));
  const desiredWidth = measureMenuWidth(
    state.menu,
    entries,
    ROOT_MENU_FALLBACK_WIDTH,
    MIN_ROOT_MENU_WIDTH,
  );
  const availableWidth = Math.max(
    0,
    Math.floor(window.innerWidth - VIEWPORT_MARGIN * 2),
  );

  state.menu.style.inlineSize = Math.min(desiredWidth, availableWidth) + "px";
}

function measureModelSubmenuWidth(state, group) {
  const contentWidth = measureMenuWidth(
    state.submenu,
    group.models.map((model) => ({ label: model.label, text: model.leaf })),
    MODEL_SUBMENU_FALLBACK_WIDTH,
    MIN_MODEL_SUBMENU_WIDTH,
  );

  return Math.max(
    state.menu.offsetWidth,
    contentWidth + MODEL_SUBMENU_TEXT_BUFFER,
  );
}

function getModelMenuDetails(menu) {
  const ownerId = menu.getAttribute("aria-labelledby");
  const submenuTrigger = ownerId ? document.getElementById(ownerId) : null;

  if (!submenuTrigger?.matches('[role="menuitem"][aria-haspopup="menu"]')) {
    return null;
  }

  const parentMenu = submenuTrigger.closest('[role="menu"]');
  if (!parentMenu) return null;

  // 设置菜单中只有第一个原生子菜单是模型列表；推理强度和速度
  // 必须继续使用 Codex 原生的单层选项，不能进入前缀分类逻辑。
  const settingsSubmenuTriggers = getDirectMenuItems(parentMenu).filter(
    (item) => item.matches('[aria-haspopup="menu"]'),
  );
  if (settingsSubmenuTriggers[0] !== submenuTrigger) return null;

  const parentOwnerId = parentMenu?.getAttribute("aria-labelledby");
  const intelligenceTrigger = parentOwnerId
    ? document.getElementById(parentOwnerId)
    : null;

  if (!intelligenceTrigger?.matches('[data-codex-intelligence-trigger="true"]')) {
    return null;
  }

  const models = getDirectMenuItems(menu)
    .map((item) => {
      const fullName = item.innerText.trim().split(/\r?\n/, 1)[0]?.trim() ?? "";
      const slashIndex = fullName.indexOf("/");
      const label = getLabelElement(item, fullName);

      if (!fullName || !label) return null;

      const prefix =
        slashIndex > 0 ? fullName.slice(0, slashIndex).trim() : null;
      const leaf =
        slashIndex > 0 ? fullName.slice(slashIndex + 1).trim() : fullName;

      if (!leaf) return null;

      return {
        item,
        label,
        fullName,
        originalTitle: item.getAttribute("title"),
        prefix,
        leaf,
        anchor: null,
      };
    })
    .filter(Boolean);

  return models.length ? { models } : null;
}

function createChevron() {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 20 20");
  svg.setAttribute("width", "20");
  svg.setAttribute("height", "20");
  svg.setAttribute("fill", "none");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("data-codex-tweaks-model-chevron", "");

  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute("d", "m7.75 4.5 5.5 5.5-5.5 5.5");
  path.setAttribute("stroke", "currentColor");
  path.setAttribute("stroke-width", "1.5");
  path.setAttribute("stroke-linecap", "round");
  path.setAttribute("stroke-linejoin", "round");
  svg.append(path);

  return svg;
}

function createFolderIcon() {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 20 20");
  svg.setAttribute("width", "20");
  svg.setAttribute("height", "20");
  svg.setAttribute("fill", "none");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("data-codex-tweaks-model-folder", "");

  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute(
    "d",
    "M3.25 6.25c0-.83.67-1.5 1.5-1.5h3l1.5 1.75h6c.83 0 1.5.67 1.5 1.5v6.25c0 .83-.67 1.5-1.5 1.5H4.75c-.83 0-1.5-.67-1.5-1.5v-8Z",
  );
  path.setAttribute("stroke", "currentColor");
  path.setAttribute("stroke-width", "1.35");
  path.setAttribute("stroke-linecap", "round");
  path.setAttribute("stroke-linejoin", "round");
  svg.append(path);

  return svg;
}

function createMenuRow(sampleItem) {
  const row = document.createElement("div");
  row.className = sampleItem.getAttribute("class") ?? "";
  row.setAttribute("role", "menuitem");
  row.setAttribute("tabindex", "-1");
  row.setAttribute("data-orientation", "vertical");
  row.setAttribute("data-codex-tweaks-model-category", "");
  return row;
}

function focusRow(row) {
  requestAnimationFrame(() => {
    if (row?.isConnected && row.getClientRects().length > 0) {
      row.focus({ preventScroll: true });
    }
  });
}

function clearModelMenuTimers(state) {
  if (state.openTimer !== null) {
    clearTimeout(state.openTimer);
    state.openTimer = null;
  }
  if (state.closeTimer !== null) {
    clearTimeout(state.closeTimer);
    state.closeTimer = null;
  }
}

function returnModelsToRoot(state) {
  for (const model of state.models) {
    if (model.anchor?.isConnected && model.item.parentElement === state.submenu) {
      model.anchor.after(model.item);
    }
    model.label.textContent = model.fullName;
    model.item.removeAttribute("data-codex-tweaks-model-active");
  }
}

function positionModelSubmenu(state, category) {
  if (state.submenu.hidden || !category?.isConnected) return;

  const menuRect = state.menu.getBoundingClientRect();
  const categoryRect = category.getBoundingClientRect();
  const activeGroup = state.groups.get(state.activeGroupKey);
  if (!activeGroup) return;

  const desiredWidth = measureModelSubmenuWidth(state, activeGroup);
  const rightAvailable =
    window.innerWidth - menuRect.right - SUBMENU_GAP - VIEWPORT_MARGIN;
  const leftAvailable = menuRect.left - SUBMENU_GAP - VIEWPORT_MARGIN;
  const placeRight =
    rightAvailable >= desiredWidth ||
    (rightAvailable >= leftAvailable && leftAvailable < desiredWidth);
  const viewportWidth = Math.max(
    0,
    Math.floor(window.innerWidth - VIEWPORT_MARGIN * 2),
  );
  const submenuWidth = Math.min(desiredWidth, viewportWidth);
  const idealLeft = placeRight
    ? menuRect.right + SUBMENU_GAP
    : menuRect.left - SUBMENU_GAP - submenuWidth;
  const maximumLeft = Math.max(
    VIEWPORT_MARGIN,
    window.innerWidth - VIEWPORT_MARGIN - submenuWidth,
  );
  const viewportLeft = Math.min(
    maximumLeft,
    Math.max(VIEWPORT_MARGIN, idealLeft),
  );

  state.submenu.dataset.codexTweaksModelSide = placeRight ? "right" : "left";
  state.submenu.toggleAttribute(
    "data-codex-tweaks-model-constrained",
    submenuWidth < desiredWidth,
  );
  state.submenu.style.inlineSize = submenuWidth + "px";
  state.submenu.style.left = viewportLeft - menuRect.left + "px";
  state.submenu.style.right = "auto";

  let top = categoryRect.top - menuRect.top;
  state.submenu.style.top = top + "px";

  let submenuRect = state.submenu.getBoundingClientRect();
  if (submenuRect.bottom > window.innerHeight - VIEWPORT_MARGIN) {
    top -=
      submenuRect.bottom - (window.innerHeight - VIEWPORT_MARGIN);
  }
  submenuRect = state.submenu.getBoundingClientRect();
  if (submenuRect.top < VIEWPORT_MARGIN) {
    top += VIEWPORT_MARGIN - submenuRect.top;
  }

  state.submenu.style.top = top + "px";
}

function hideModelSubmenu(state, focusGroupKey = null) {
  clearModelMenuTimers(state);
  const previousGroupKey = state.activeGroupKey;

  returnModelsToRoot(state);
  state.activeGroupKey = null;
  state.submenu.hidden = true;
  state.submenu.removeAttribute("aria-label");

  for (const [groupKey, category] of state.categories) {
    category.setAttribute("aria-expanded", "false");
    category.removeAttribute("data-highlighted");
    if (groupKey === (focusGroupKey ?? previousGroupKey)) {
      focusRow(category);
    }
  }
}

function showModelGroup(state, groupKey, focusFirst = false) {
  if (!state.menu.isConnected) return;

  clearModelMenuTimers(state);
  const group = state.groups.get(groupKey);
  const category = state.categories.get(groupKey);
  if (!group || !category) return;

  if (state.activeGroupKey !== groupKey) {
    returnModelsToRoot(state);
    state.activeGroupKey = groupKey;

    for (const [candidateKey, candidate] of state.categories) {
      const isActive = candidateKey === groupKey;
      candidate.setAttribute("aria-expanded", String(isActive));
      candidate.toggleAttribute("data-highlighted", isActive);
    }

    state.submenu.setAttribute("aria-label", group.label);
    for (const model of group.models) {
      model.label.textContent = model.leaf;
      model.item.setAttribute("data-codex-tweaks-model-active", "");
      state.submenu.append(model.item);
    }
  }

  state.submenu.hidden = false;
  positionModelSubmenu(state, category);
  if (focusFirst) focusRow(group.models[0]?.item);
}

function scheduleModelGroup(state, groupKey) {
  clearModelMenuTimers(state);
  state.openTimer = setTimeout(() => {
    state.openTimer = null;
    showModelGroup(state, groupKey);
  }, HOVER_OPEN_DELAY);
}

function scheduleModelSubmenuClose(state) {
  if (state.closeTimer !== null) clearTimeout(state.closeTimer);
  state.closeTimer = setTimeout(() => {
    state.closeTimer = null;
    hideModelSubmenu(state);
  }, HOVER_CLOSE_DELAY);
}

function getKeyboardRows(state, eventTarget) {
  const container = state.submenu.contains(eventTarget)
    ? state.submenu
    : state.wrapper;

  return [...container.children].filter(
    (row) =>
      row.matches('[role="menuitem"]') && row.getClientRects().length > 0,
  );
}

function handleEnhancedMenuKeydown(state, event) {
  const consumeEvent = () => {
    event.preventDefault();
    event.stopImmediatePropagation();
  };
  const focusedCategory = event.target.closest?.(
    "[data-codex-tweaks-model-category]",
  );
  const isInSubmenu = state.submenu.contains(event.target);

  if (
    state.activeGroupKey &&
    (isInSubmenu || focusedCategory) &&
    (event.key === "ArrowLeft" || event.key === "Escape")
  ) {
    consumeEvent();
    hideModelSubmenu(state, state.activeGroupKey);
    return;
  }

  if (
    focusedCategory &&
    (event.key === "ArrowRight" ||
      event.key === "Enter" ||
      event.key === " ")
  ) {
    consumeEvent();
    showModelGroup(
      state,
      focusedCategory.dataset.codexTweaksModelGroup,
      true,
    );
    return;
  }

  if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
    return;
  }

  const rows = getKeyboardRows(state, event.target);
  if (!rows.length) return;

  consumeEvent();

  const currentIndex = rows.indexOf(document.activeElement);
  let nextIndex;

  if (event.key === "Home") nextIndex = 0;
  else if (event.key === "End") nextIndex = rows.length - 1;
  else if (event.key === "ArrowDown") {
    nextIndex = currentIndex < 0 ? 0 : (currentIndex + 1) % rows.length;
  } else {
    nextIndex =
      currentIndex < 0
        ? rows.length - 1
        : (currentIndex - 1 + rows.length) % rows.length;
  }

  rows[nextIndex].focus({ preventScroll: true });
}

function handleModelMenuKeydown(event) {
  const menu = event.target?.closest?.("[" + MODEL_MENU_MARKER + "]");
  const state = menu ? modelMenuStates.get(menu) : null;
  if (state) handleEnhancedMenuKeydown(state, event);
}

function handleModelMenuViewportChange() {
  for (const state of modelMenuStates.values()) {
    sizeRootModelMenu(state);
    if (!state.activeGroupKey || state.submenu.hidden) continue;
    positionModelSubmenu(
      state,
      state.categories.get(state.activeGroupKey),
    );
  }
}

function enhanceModelMenu(menu, details) {
  if (menu.hasAttribute(MODEL_MENU_MARKER)) return;

  const wrapper = details.models[0]?.item.parentElement;
  if (!wrapper) return;

  const groups = new Map();
  for (const model of details.models) {
    const groupKey = model.prefix
      ? "prefix:" + model.prefix
      : DEFAULT_GROUP_KEY;
    const groupLabel = model.prefix ?? DEFAULT_GROUP_LABEL;
    let group = groups.get(groupKey);

    if (!group) {
      group = { key: groupKey, label: groupLabel, models: [] };
      groups.set(groupKey, group);
    }
    group.models.push(model);

    model.anchor = document.createComment("codex-tweaks-model-anchor");
    model.item.before(model.anchor);
    model.item.setAttribute("data-codex-tweaks-model-kind", "model");
    model.label.setAttribute("data-codex-tweaks-model-label", "");
    model.item.title = model.fullName;
  }

  menu.setAttribute(MODEL_MENU_MARKER, "");

  const sampleItem = details.models[0].item;
  const categories = new Map();

  for (const group of groups.values()) {
    const category = createMenuRow(sampleItem);
    category.dataset.codexTweaksModelGroup = group.key;
    category.setAttribute("aria-haspopup", "menu");
    category.setAttribute("aria-expanded", "false");
    category.setAttribute(
      "aria-label",
      group.label + "，" + group.models.length + " 个模型",
    );
    category.title = group.label;

    const content = document.createElement("div");
    content.className = "flex w-full min-w-0 items-center gap-1.5";

    const label = document.createElement("span");
    label.className = "flex-1 min-w-0 truncate";
    label.textContent = group.label;
    label.setAttribute("data-codex-tweaks-model-category-label", "");

    const count = document.createElement("span");
    count.textContent = String(group.models.length);
    count.setAttribute("data-codex-tweaks-model-count", "");
    count.setAttribute("aria-hidden", "true");

    content.append(
      createFolderIcon(),
      label,
      count,
      createChevron(),
    );
    category.append(content);
    group.models[0].anchor.before(category);
    categories.set(group.key, category);
  }

  const submenu = document.createElement("div");
  submenu.className = menu.getAttribute("class") ?? "";
  submenu.hidden = true;
  submenu.setAttribute("role", "menu");
  submenu.setAttribute("aria-orientation", "vertical");
  submenu.setAttribute("tabindex", "-1");
  submenu.setAttribute(MODEL_SUBMENU_MARKER, "");
  menu.append(submenu);

  const state = {
    menu,
    wrapper,
    models: details.models,
    groups,
    categories,
    submenu,
    activeGroupKey: null,
    openTimer: null,
    closeTimer: null,
    onRootScroll: null,
  };
  modelMenuStates.set(menu, state);
  sizeRootModelMenu(state);

  for (const [groupKey, category] of categories) {
    category.addEventListener("pointerenter", () =>
      scheduleModelGroup(state, groupKey),
    );
    category.addEventListener("pointerleave", () =>
      scheduleModelSubmenuClose(state),
    );
    category.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      showModelGroup(state, groupKey, true);
    });
  }

  submenu.addEventListener("pointerenter", () => clearModelMenuTimers(state));
  submenu.addEventListener("pointerleave", () =>
    scheduleModelSubmenuClose(state),
  );

  state.onRootScroll = () => {
    if (!state.activeGroupKey || state.submenu.hidden) return;
    positionModelSubmenu(
      state,
      state.categories.get(state.activeGroupKey),
    );
  };
  wrapper.addEventListener("scroll", state.onRootScroll, { passive: true });
}

function restoreModelMenu(state) {
  clearModelMenuTimers(state);
  returnModelsToRoot(state);
  state.wrapper.removeEventListener("scroll", state.onRootScroll);
  state.submenu.remove();

  for (const category of state.categories.values()) category.remove();

  for (const model of state.models) {
    model.label.textContent = model.fullName;
    model.item.removeAttribute("data-codex-tweaks-model-kind");
    model.item.removeAttribute("data-codex-tweaks-model-active");
    model.label.removeAttribute("data-codex-tweaks-model-label");
    if (model.originalTitle === null) model.item.removeAttribute("title");
    else model.item.setAttribute("title", model.originalTitle);
    model.anchor?.remove();
  }

  state.menu.removeAttribute(MODEL_MENU_MARKER);
}

function scanForModelMenus() {
  modelMenuScanQueued = false;

  for (const [menu, state] of modelMenuStates) {
    if (!menu.isConnected) {
      clearModelMenuTimers(state);
      modelMenuStates.delete(menu);
      continue;
    }

    const isStale =
      !state.wrapper.isConnected ||
      state.models.some(
        (model) =>
          !model.anchor?.isConnected || !state.menu.contains(model.item),
      );

    if (isStale) {
      restoreModelMenu(state);
      modelMenuStates.delete(menu);
    }
  }

  for (const menu of document.querySelectorAll(
    '[role="menu"]:not([data-codex-tweaks-model-menu])',
  )) {
    const details = getModelMenuDetails(menu);
    if (details) enhanceModelMenu(menu, details);
  }
}

function queueModelMenuScan() {
  if (modelMenuScanQueued) return;
  modelMenuScanQueued = true;
  queueMicrotask(scanForModelMenus);
}

const modelMenuObserver = new MutationObserver(queueModelMenuScan);
modelMenuObserver.observe(document.body, { childList: true, subtree: true });
window.addEventListener("keydown", handleModelMenuKeydown, true);
window.addEventListener("resize", handleModelMenuViewportChange);
scanForModelMenus();

api.registerCleanup(() => {
  modelMenuObserver.disconnect();
  window.removeEventListener("keydown", handleModelMenuKeydown, true);
  window.removeEventListener("resize", handleModelMenuViewportChange);
  for (const state of modelMenuStates.values()) restoreModelMenu(state);
  modelMenuStates.clear();
});
