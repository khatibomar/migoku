(async () => {
  const payload = __PAYLOAD__;
  const actionLabel = (payload.actionLabel || "").trim();
  if (!actionLabel) {
    return { ok: false, reason: "invalid_status" };
  }

  const rawItems =
    Array.isArray(payload.items) && payload.items.length
      ? payload.items
      : [{ wordText: payload.wordText, secondary: payload.secondary }];
  const items = rawItems
    .map((item) => ({
      wordText: (item?.wordText || "").trim(),
      secondary: (item?.secondary || "").trim(),
    }))
    .filter((item) => item.wordText);
  if (!items.length) {
    return { ok: false, reason: "missing_word_text" };
  }

  const searchValue = [...new Set(items.map((item) => item.wordText))].join(
    " ",
  );
  if (!searchValue) {
    return { ok: false, reason: "missing_word_text" };
  }

  const raf = () => new Promise(requestAnimationFrame);
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const timeoutMs = 12000;
  const rowTimeoutMs = 3000;
  const listSettleIdleMs = 150;
  const listSettleTimeoutMs = rowTimeoutMs;
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

  const matchesRow = (row, item) => {
    const primaryText =
      row
        .querySelector(".WordBrowserList__labelContainer .-emphasis")
        ?.textContent?.trim() || "";
    const secondaryText =
      row.querySelector(".WordBrowserList__secondary")?.textContent?.trim() ||
      "";

    if (item.secondary) {
      return primaryText === item.wordText && secondaryText === item.secondary;
    }
    return primaryText === item.wordText;
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
      timeoutId = setTimeout(() => finish(null), listSettleTimeoutMs);
    });

  const findRow = (item) =>
    [...document.querySelectorAll(".UiDynamicList__item")].find((row) =>
      matchesRow(row, item),
    );

  const waitForRowOrSettle = async (item) => {
    const settled = waitForListSettle().then(() => null);
    const row = await Promise.race([
      waitFor(() => findRow(item), rowTimeoutMs),
      settled,
    ]);
    return row || findRow(item);
  };

  const isGrey = (el) => {
    const button = el.closest("button");
    const textStyle = el.getAttribute("style") || "";
    const buttonStyle = button?.getAttribute("style") || "";
    return (
      textStyle.includes("var(--grey-4)") ||
      buttonStyle.includes("var(--grey-4)")
    );
  };

  const results = [];
  let hadRetry = false;
  for (const item of items) {
    let row = await waitForRowOrSettle(item);
    if (!row && !hadRetry) {
      await applySearchValue();
      row = await waitForRowOrSettle(item);
      hadRetry = true;
    }
    if (!row) {
      results.push({
        wordText: item.wordText,
        secondary: item.secondary,
        ok: false,
        reason: "row_not_found",
      });
      continue;
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
      results.push({
        wordText: item.wordText,
        secondary: item.secondary,
        ok: false,
        reason: "change_status_button_not_found",
      });
      continue;
    }

    changeStatusButton.click();

    const actionText = await waitFor(() => {
      const matches = [...document.querySelectorAll(".UiActionSheet__item__text")]
        .filter((el) => el.textContent?.trim() === actionLabel)
        .filter(isVisible);
      return matches[matches.length - 1];
    });
    if (!actionText) {
      results.push({
        wordText: item.wordText,
        secondary: item.secondary,
        ok: false,
        reason: "action_not_found",
      });
      continue;
    }

    await raf();
    if (isGrey(actionText)) {
      results.push({
        wordText: item.wordText,
        secondary: item.secondary,
        ok: false,
        reason: "action_disabled",
      });
      continue;
    }

    actionText.closest("button")?.click();
    results.push({
      wordText: item.wordText,
      secondary: item.secondary,
      ok: true,
    });
  }

  const ok = results.length > 0 && results.every((result) => result.ok);
  return { ok, results };
})();
