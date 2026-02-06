(() => {
  const email = document.querySelector('input[type="email"][name="email"], input#email');
  const password = document.querySelector('input[type="password"][name="password"], input#password');
  const hasError = (el) => !!el && (el.classList.contains('-error') || el.classList.contains('error'));
  return hasError(email) || hasError(password);
})();
