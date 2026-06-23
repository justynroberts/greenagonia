/* ============================================================
   Greenagonia — shared page chrome
   Injects the header, footer, cart drawer, checkout modal,
   operations console and toast container into every page so
   the markup lives in exactly one place.
   ============================================================ */

const CHROME_TOP = `
<header class="header" id="header">
  <a href="index.html" class="logo" id="header-logo" title="Double-click to configure PagerDuty">
    <svg class="logo__mark" viewBox="0 0 32 32" fill="none">
      <path d="M16 3 L26 22 H20 L24 29 H8 L12 22 H6 Z" fill="currentColor"/>
    </svg>
    <span class="logo__text">Greenagonia</span>
  </a>
  <nav class="nav">
    <a href="index.html" class="nav__link" data-nav="home">Home</a>
    <a href="shop.html" class="nav__link" data-nav="shop">Shop</a>
    <a href="index.html#story" class="nav__link">Our Story</a>
    <a href="index.html#impact" class="nav__link">Impact</a>
  </nav>
  <div class="header__actions">
    <button class="icon-btn cart-btn" id="cart-btn" aria-label="Open cart">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4Z"/>
        <path d="M3 6h18"/>
        <path d="M16 10a4 4 0 0 1-8 0"/>
      </svg>
      <span class="cart-btn__badge" id="cart-badge" hidden>0</span>
    </button>
  </div>
</header>`;

const CHROME_BOTTOM = `
<footer class="footer">
  <div class="footer__brand" id="footer-brand" title="">
    <svg class="logo__mark" viewBox="0 0 32 32" fill="none"><path d="M16 3 L26 22 H20 L24 29 H8 L12 22 H6 Z" fill="currentColor"/></svg>
    Greenagonia
  </div>
  <p>© 2026 Greenagonia Outfitters. Built to outlast the trail.</p>
  <p class="footer__fineprint"><a href="#" id="ops-link">operations console</a></p>
</footer>

<!-- cart drawer -->
<div class="overlay" id="cart-overlay"></div>
<aside class="drawer" id="cart-drawer" aria-label="Shopping cart">
  <div class="drawer__head">
    <h3>Your Pack <span id="cart-count-label"></span></h3>
    <button class="icon-btn" id="cart-close" aria-label="Close cart">✕</button>
  </div>
  <div class="drawer__body" id="cart-items"></div>
  <div class="drawer__foot" id="cart-foot">
    <div class="cart-totals">
      <div class="cart-totals__row"><span>Subtotal</span><span id="cart-subtotal">$0.00</span></div>
      <div class="cart-totals__row"><span>Carbon-neutral shipping</span><span class="text-green">Free</span></div>
      <div class="cart-totals__row cart-totals__row--total"><span>Total</span><span id="cart-total">$0.00</span></div>
    </div>
    <button class="btn btn--primary btn--block" id="checkout-btn">Checkout securely</button>
    <p class="drawer__note">🌲 This order plants <strong id="trees-count">3</strong> trees</p>
  </div>
</aside>`;

/* The checkout UI. Rendered inline into #inline-checkout when the page
   provides one (the front page hero), otherwise wrapped in a modal. */
const CHECKOUT_STEPS = `
    <div class="checkout-step" id="step-form">
      <h3 class="modal__title">Checkout</h3>
      <p class="modal__sub">Order <span class="mono" id="order-id-label"></span></p>
      <form id="checkout-form" autocomplete="off">
        <div class="form-row">
          <label>Full name<input type="text" id="cust-name" value="Alex Rivers" required></label>
        </div>
        <div class="form-row">
          <label>Email<input type="email" id="cust-email" value="alex@example.com" required></label>
        </div>
        <div class="form-row form-row--split">
          <label>Card number<input type="text" id="cust-card" value="4242 4242 4242 4242" maxlength="19" required></label>
          <label>Expiry<input type="text" id="cust-exp" value="12/28" maxlength="5" required></label>
          <label>CVC<input type="text" id="cust-cvc" value="424" maxlength="4" required></label>
        </div>
        <div class="checkout-summary" id="checkout-summary"></div>
        <button type="submit" class="btn btn--primary btn--block btn--lg" id="pay-btn">
          Pay <span id="pay-amount"></span>
        </button>
        <p class="secure-note">🔒 256-bit encrypted · PCI DSS compliant</p>
      </form>
    </div>

    <div class="checkout-step" id="step-processing" hidden>
      <h3 class="modal__title">Processing your order…</h3>
      <ul class="pipeline" id="pipeline">
        <li class="pipeline__step" data-step="validate"><span class="pipeline__icon"></span><span>Validating cart</span><span class="pipeline__time mono"></span></li>
        <li class="pipeline__step" data-step="inventory"><span class="pipeline__icon"></span><span>Reserving inventory</span><span class="pipeline__time mono"></span></li>
        <li class="pipeline__step" data-step="payment"><span class="pipeline__icon"></span><span>Charging payment</span><span class="pipeline__time mono"></span></li>
        <li class="pipeline__step" data-step="order"><span class="pipeline__icon"></span><span>Creating order</span><span class="pipeline__time mono"></span></li>
        <li class="pipeline__step" data-step="confirm"><span class="pipeline__icon"></span><span>Sending confirmation</span><span class="pipeline__time mono"></span></li>
      </ul>
    </div>

    <div class="checkout-step checkout-step--center" id="step-success" hidden>
      <div class="result-icon result-icon--ok">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>
      </div>
      <h3 class="modal__title">Order confirmed!</h3>
      <p class="modal__sub">Order <span class="mono" id="success-order-id"></span> is on its way.<br>Three trees will be planted in your name. 🌲🌲🌲</p>
      <button class="btn btn--primary" id="success-done">Keep exploring</button>
    </div>

    <div class="checkout-step checkout-step--center" id="step-failure" hidden>
      <div class="result-icon result-icon--fail">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>
      </div>
      <h3 class="modal__title" id="failure-title">Something went wrong</h3>
      <p class="modal__sub" id="failure-message">We couldn't complete your order.</p>
      <div class="failure-meta mono" id="failure-meta"></div>
      <div class="failure-actions">
        <button class="btn btn--primary" id="failure-retry">Try again</button>
        <button class="btn btn--ghost" id="failure-close">Back to cart</button>
      </div>
      <p class="failure-pd" id="failure-pd-note" hidden>📟 Incident dispatched to the on-call engineer via PagerDuty</p>
    </div>`;

/* Operations console + toasts */
const CHROME_PANELS = `
<div class="overlay" id="chaos-overlay"></div>
<aside class="chaos" id="chaos-panel" aria-label="Operations console">
  <div class="chaos__head">
    <div>
      <h3 class="chaos__title">⚡ Operations Console</h3>
      <p class="chaos__sub">Inject failures · dispatch PagerDuty alerts</p>
    </div>
    <button class="icon-btn" id="chaos-close" aria-label="Close console">✕</button>
  </div>

  <div class="chaos__body">
    <section class="chaos__section">
      <h4>PagerDuty integration</h4>
      <label class="chaos__field">
        <span>Events API v2 routing key</span>
        <input type="password" id="pd-routing-key" placeholder="32-char integration key" class="mono">
      </label>
      <label class="chaos__field">
        <span>Change event key — GitHub deploys</span>
        <input type="password" id="pd-change-key" placeholder="service integration key" class="mono">
      </label>
      <label class="chaos__field">
        <span>Change event key — LaunchDarkly flags</span>
        <input type="password" id="pd-ld-key" placeholder="service integration key" class="mono">
      </label>
      <div class="chaos__row">
        <button class="btn btn--sm btn--ghost" id="pd-test">Send test alert</button>
        <span class="chaos__status" id="pd-status">No key configured</span>
      </div>
    </section>

    <section class="chaos__section">
      <h4>Failure scenarios</h4>
      <p class="chaos__hint">Select a scenario below, then either run a checkout or click <strong>Fire now</strong> to inject alerts directly.</p>
      <div class="scenario-list" id="scenario-list"></div>
      <div class="chaos__row" style="margin-top:8px">
        <button class="btn btn--sm btn--primary" id="pd-fire-cascade">Fire now</button>
      </div>
    </section>

    <section class="chaos__section">
      <h4>Active incidents <span class="chip" id="incident-count">0</span></h4>
      <div class="chaos__row">
        <button class="btn btn--sm btn--ghost" id="pd-resolve-all">Resolve all</button>
        <button class="btn btn--sm btn--ghost" id="log-clear">Clear log</button>
      </div>
      <div class="event-log mono" id="event-log">
        <div class="event-log__empty">No events dispatched yet</div>
      </div>
    </section>
  </div>
</aside>

<div class="toasts" id="toasts"></div>

<!-- PagerDuty routing key setup -->
<div class="modal-backdrop" id="pdkey-modal" hidden>
  <div class="modal pdkey" role="dialog" aria-modal="true" aria-label="PagerDuty setup">
    <div class="pdkey__icon">📟</div>
    <h3 class="modal__title">Connect PagerDuty</h3>
    <p class="modal__sub">Paste an <strong>Events API v2</strong> routing key so simulated checkout failures can page the on-call.<br>Stored only in this browser.</p>
    <form id="pdkey-form">
      <div class="form-row">
        <label>Routing key<input type="text" id="pdkey-input" class="mono" placeholder="32-character integration key" autocomplete="off" spellcheck="false"></label>
      </div>
      <button type="submit" class="btn btn--primary btn--block">Save key</button>
    </form>
    <div class="pdkey__actions">
      <button class="btn btn--ghost btn--sm" id="pdkey-skip">Continue without alerts</button>
      <button class="btn btn--ghost btn--sm" id="pdkey-clear" hidden>Remove saved key</button>
    </div>
    <p class="secure-note">Double-click the Greenagonia logo any time to change this.</p>
  </div>
</div>`;

// Scripts sit at the end of <body>, so the page content already exists.
document.body.insertAdjacentHTML("afterbegin", CHROME_TOP);
document.body.insertAdjacentHTML("beforeend", CHROME_BOTTOM);
document.body.insertAdjacentHTML("beforeend", CHROME_PANELS);

// Checkout: inline on pages that provide a mount, modal everywhere else.
const inlineCheckout = document.getElementById("inline-checkout");
if (inlineCheckout) {
  inlineCheckout.innerHTML = CHECKOUT_STEPS;
} else {
  document.body.insertAdjacentHTML(
    "beforeend",
    `<div class="modal-backdrop" id="checkout-modal" hidden>
      <div class="modal" role="dialog" aria-modal="true" aria-label="Checkout">
        <button class="icon-btn modal__close" id="checkout-close" aria-label="Close checkout">✕</button>
        ${CHECKOUT_STEPS}
      </div>
    </div>`
  );
}

// highlight the active nav link
const activeNav = document.body.dataset.page;
if (activeNav) {
  const link = document.querySelector(`[data-nav="${activeNav}"]`);
  if (link) link.style.color = "var(--green-bright)";
}
