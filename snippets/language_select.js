(() => {
  const raw = __LANG__;
  const normalized = raw.trim().toLowerCase();
  if (!normalized) {
    return { clicked: false, method: "missing" };
  }

  const code = normalized.split(/[\s,;:.]/)[0].split(/[_-]/)[0];
  const candidates = Array.from(new Set([normalized, code].filter(Boolean)));

  for (const candidate of candidates) {
    const button = document.querySelector(
      'button[aria-label="ID:LanguageSelect.' + candidate + '"]',
    );
    if (button) {
      button.click();
      return { clicked: true, method: "aria" };
    }
  }

  const options = [
    ...document.querySelectorAll("button.LanguageSelect__option"),
  ];
  const match = options.find((button) => {
    const label = button.querySelector(".LanguageInfo .UiTypo");
    const text = label?.textContent?.trim().toLowerCase() || "";
    return text === normalized;
  });

  if (match) {
    match.click();
    return { clicked: true, method: "text" };
  }

  return { clicked: false, method: "not_found" };
})();
