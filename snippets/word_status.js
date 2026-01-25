(async () => {
  const payload = __PAYLOAD__;
  const actionLabel = (payload.actionLabel || "").trim();
  if (!actionLabel) {
    return { ok: false, reason: "invalid_status" };
  }

  const wordText = (payload.wordText || "").trim();
  const secondary = (payload.secondary || "").trim();
  const searchValue = (wordText || secondary || "").trim();
  if (!searchValue) {
    return { ok: false, reason: "missing_search_term" };
  }

  const raf = () => new Promise(requestAnimationFrame);
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const timeoutMs = 12000;
  const rowTimeoutMs = 3000;
  const listSettleIdleMs = 150;
  const listSettleTimeoutMs = 1200;
  const waitFor = async (fn, limitMs = timeoutMs) => {
    const start = performance.now();
    while (performance.now() - start < limitMs) {
      const value = fn();
      if (value) return value;
      await raf();
    }
    return null;
  };

  const isVisible = (el) => {
    if (!el) return false;
    const style = getComputedStyle(el);
    return (
      style.display !== "none" &&
      style.visibility !== "hidden" &&
      el.getClientRects().length > 0
    );
  };

  const ensureSearchInput = async () => {
    const getInput = () => document.querySelector("#search-input-field");
    const existing = getInput();
    if (!existing) {
      const button = await waitFor(() =>
        document.querySelector(
          'button[title="Search"], button[aria-label="Search"]',
        ),
      );
      if (!button) return null;
      button.click();
    }

    const input = await waitFor(() => {
      const el = getInput();
      if (!isVisible(el)) return null;
      if (el.disabled || el.readOnly) return null;
      return el;
    });

    if (!input) return null;
    input.focus();
    await waitFor(() => document.activeElement === input);
    return input;
  };

  const input = await ensureSearchInput();
  if (!input) {
    return { ok: false, reason: "search_input_timeout" };
  }

  const nativeValueSetter = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )?.set;
  const setValue = (value) => {
    if (nativeValueSetter) {
      nativeValueSetter.call(input, value);
    } else {
      input.value = value;
    }
    input.dispatchEvent(new Event("input", { bubbles: true }));
  };
  const applySearchValue = async () => {
    const bump = "__migoku__" + Math.random().toString(36).slice(2, 8);
    setValue(bump);
    await sleep(240);
    setValue(searchValue);
  };
  await applySearchValue();

  const matchesRow = (row) => {
    const primaryText =
      row
        .querySelector(".WordBrowserList__labelContainer .-emphasis")
        ?.textContent?.trim() || "";
    const secondaryText =
      row.querySelector(".WordBrowserList__secondary")?.textContent?.trim() ||
      "";

    if (wordText && secondary) {
      return primaryText === wordText && secondaryText === secondary;
    }
    if (wordText) {
      return primaryText === wordText;
    }
    if (secondary) {
      return secondaryText === secondary;
    }
    return false;
  };

  const waitForListSettle = () =>
    new Promise((resolve) => {
      const wrapper = document.querySelector(
        ".vue-recycle-scroller__item-wrapper",
      );
      if (!wrapper) {
        setTimeout(() => resolve(null), listSettleTimeoutMs);
        return;
      }

      let settledTimer;
      let timeoutId;
      let finished = false;
      let observer;

      const finish = (value) => {
        if (finished) return;
        finished = true;
        if (settledTimer) clearTimeout(settledTimer);
        if (timeoutId) clearTimeout(timeoutId);
        if (observer) observer.disconnect();
        resolve(value);
      };

      observer = new MutationObserver(() => {
        if (settledTimer) clearTimeout(settledTimer);
        settledTimer = setTimeout(() => finish(true), listSettleIdleMs);
      });
      observer.observe(wrapper, {
        childList: true,
        subtree: true,
        characterData: true,
      });
      settledTimer = setTimeout(() => finish(true), listSettleIdleMs);
      timeoutId = setTimeout(() => finish(null), listSettleTimeoutMs);
    });

  const findRow = () =>
    [...document.querySelectorAll(".UiDynamicList__item")].find(matchesRow);

  const waitForRowOrSettle = async () => {
    const settled = waitForListSettle().then(() => null);
    const row = await Promise.race([waitFor(findRow, rowTimeoutMs), settled]);
    return row || findRow();
  };

  let row = await waitForRowOrSettle();
  if (!row) {
    await applySearchValue();
    row = await waitForRowOrSettle();
  }
  if (!row) {
    return { ok: false, reason: "row_not_found" };
  }

  row.scrollIntoView({ block: "center" });
  row.click();

  const changeStatusButton = await waitFor(() => {
    const label = [
      ...document.querySelectorAll("button .UiTypo__buttonText"),
    ].find((el) => el.textContent?.trim() === "Change status");
    const button = label?.closest("button");
    if (!button) return null;
    if (button.disabled || button.getAttribute("aria-disabled") === "true")
      return null;
    return button;
  });
  if (!changeStatusButton) {
    return { ok: false, reason: "change_status_button_not_found" };
  }

  changeStatusButton.click();

  const actionText = await waitFor(() => {
    const matches = [...document.querySelectorAll(".UiActionSheet__item__text")]
      .filter((el) => el.textContent?.trim() === actionLabel)
      .filter(isVisible);
    return matches[matches.length - 1];
  });
  if (!actionText) {
    return { ok: false, reason: "action_not_found" };
  }

  const isGrey = (el) => {
    const button = el.closest("button");
    const textStyle = el.getAttribute("style") || "";
    const buttonStyle = button?.getAttribute("style") || "";
    return (
      textStyle.includes("var(--grey-4)") ||
      buttonStyle.includes("var(--grey-4)")
    );
  };

  await raf();
  if (isGrey(actionText)) {
    return { ok: false, reason: "action_disabled" };
  }

  actionText.closest("button")?.click();
  return { ok: true };
})();
