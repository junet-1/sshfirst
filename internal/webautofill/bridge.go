// Package webautofill installs the small, non-secret browser-side bridge used
// by embedded web panels. The bridge is injected into every future iframe by
// the webview itself, which is the only way to reach a cross-origin login form
// without disabling the browser's same-origin protection.
//
// Install has a native implementation per webview engine — WebKitGTK on Linux,
// WKWebView on macOS — but both inject the identical script below.
package webautofill

// Script intentionally contains no credentials. The trusted application
// shell sends credentials with postMessage to the configured panel origin only.
// The remote page could read values after they are placed in its own inputs
// anyway; no other origin receives the message.
const Script = `(() => {
  if (window.__sshFirstAutofillBridge) return;
  window.__sshFirstAutofillBridge = true;

  const visible = (element) => {
    if (!(element instanceof HTMLElement) || element.disabled || element.readOnly) return false;
    const style = getComputedStyle(element);
    const rect = element.getBoundingClientRect();
    return style.display !== 'none' && style.visibility !== 'hidden' &&
      rect.width > 0 && rect.height > 0 && rect.right > 0 && rect.bottom > 0;
  };

  // Authentik and similar component-based login pages keep the real controls
  // inside several nested open shadow roots. querySelectorAll(document) cannot
  // see them, so collect every reachable root for each attempt.
  const openRoots = () => {
    const roots = [document];
    for (let index = 0; index < roots.length; index++) {
      for (const element of roots[index].querySelectorAll('*')) {
        if (element.shadowRoot) roots.push(element.shadowRoot);
      }
    }
    return roots;
  };

  const firstVisible = (selectors) => {
    const roots = openRoots();
    for (const selector of selectors) {
      for (const root of roots) {
        const match = Array.from(root.querySelectorAll(selector)).find(visible);
        if (match) return match;
      }
    }
    return null;
  };

  const setValue = (control, value) => {
    if (!control || !value || control.value === value) return false;
    let descriptor = null;
    if (control instanceof HTMLInputElement) {
      descriptor = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value');
    } else if (control instanceof HTMLSelectElement) {
      descriptor = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value');
    }
    // Custom login controls (notably FRITZ!Box's <password-input>) expose
    // their own value setter, so ordinary assignment is intentional here.
    if (descriptor && descriptor.set) descriptor.set.call(control, value);
    else control.value = value;
    control.dispatchEvent(new Event('input', { bubbles: true }));
    control.dispatchEvent(new Event('change', { bubbles: true }));
    return control.value === value;
  };

  const submit = (anchor) => {
    const form = anchor && anchor.closest('form');
    if (form) {
      // Prefer a real click: applications such as Technitium implement login
      // in the button's onclick handler. requestSubmit() without a submitter
      // bypasses that handler and merely reloads the document.
      const button = Array.from(form.querySelectorAll('button[type="submit"], input[type="submit"], button:not([type])')).find(visible);
      if (button) button.click();
      else if (typeof form.requestSubmit === 'function') form.requestSubmit();
      return;
    }
    const button = Array.from(document.querySelectorAll('button, input[type="submit"]')).find((candidate) => {
      const label = (candidate.innerText || candidate.value || candidate.getAttribute('aria-label') || '').trim();
      return /^(log\s?in|sign\s?in|anmelden|einloggen|weiter|next)$/i.test(label);
    });
    if (button) button.click();
  };

  const likelyIdentifierStep = (control) => {
    if (!control) return false;
    const autocomplete = (control.getAttribute('autocomplete') || '').toLowerCase();
    if (autocomplete === 'username' || autocomplete === 'email') return true;
    const form = control.closest('form');
    if (!form) return false;
    const button = Array.from(form.querySelectorAll('button[type="submit"], input[type="submit"], button:not([type])')).find(visible);
    const label = (button && (button.innerText || button.value || button.getAttribute('aria-label')) || '').trim();
    return /^(log\s?in|sign\s?in|anmelden|einloggen|weiter|next|continue)$/i.test(label);
  };

  const markSubmitted = () => {
    parent.postMessage({
      type: 'ssh-first:web-autofill-submitted',
      targetOrigin: location.origin
    }, '*');
    if (observer) observer.disconnect();
    observer = null;
    if (retryTimer) clearTimeout(retryTimer);
    retryTimer = null;
  };

  let activeData = null;
  let observer = null;
  let submitted = false;
  let nextClicked = false;
  let retryTimer = null;

  const attempt = () => {
      const data = activeData;
      if (!data || submitted) return false;
      const email = firstVisible([
        '#uiViewUser',
        'select[autocomplete="username"]',
        'select[name*="user" i]',
        'input[autocomplete="username"]',
        'input[autocomplete="email"]',
        'input[type="email"]',
        'input[name*="email" i]',
        'input[name*="user" i]',
        'input[id*="email" i]',
        'input[id*="user" i]'
      ]);
      const password = firstVisible([
        'password-input#uiPass',
        '#uiPassInput',
        'input[ta-password-type]',
        'input[autocomplete="current-password"]',
        'input[type="password"]'
      ]);

      const identifierOnly = !password && likelyIdentifierStep(email);
      const emailChanged = (password || identifierOnly) ? setValue(email, data.email) : false;
      const passwordChanged = setValue(password, data.password);
      if (password && data.password && !submitted) {
        submitted = true;
        markSubmitted();
        setTimeout(() => submit(password), 80);
      } else if (identifierOnly && email && data.email && email.value === data.email && !nextClicked) {
        // Supports two-step login pages. The app repeats the targeted message
        // after navigation so the password page is filled as well.
        nextClicked = true;
        setTimeout(() => submit(email), 120);
      }
      return emailChanged || passwordChanged;
  };

  addEventListener('message', (event) => {
    const data = event.data;
    // Two ways the credential arrives. In an iframe panel the app shell posts
    // it from the parent frame. In a native panel view the page is top-level,
    // so the app injects the message into the document itself and window,
    // parent and top are all the same object — only code already running in
    // this document can do that, and such code could read the form anyway.
    const fromParent = window !== top && event.source === parent;
    const fromSelf = window === top && event.source === window;
    if (!(fromParent || fromSelf) || !data || data.type !== 'ssh-first:web-autofill') return;
    if (data.targetOrigin !== location.origin || typeof data.email !== 'string' || typeof data.password !== 'string') return;
    if (submitted) return;

    activeData = data;
    attempt();
    if (submitted) return;
    if (observer) observer.disconnect();
    observer = new MutationObserver(() => {
      // Component frameworks often produce a burst of mutations for one stage
      // transition. Coalesce them so a large page is not scanned repeatedly.
      if (retryTimer) clearTimeout(retryTimer);
      retryTimer = setTimeout(() => {
        retryTimer = null;
        attempt();
      }, 40);
    });
    for (const root of openRoots()) {
      observer.observe(root === document ? document.documentElement : root, {
        childList: true,
        subtree: true,
        attributes: true
      });
    }
    setTimeout(() => {
      if (observer) observer.disconnect();
      observer = null;
      if (retryTimer) clearTimeout(retryTimer);
      retryTimer = null;
    }, 15000);
  });
})();`
