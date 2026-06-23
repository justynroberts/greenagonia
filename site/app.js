/* ============================================================
   Greenagonia — storefront application
   ============================================================ */

const fmt = (n) => `$${n.toFixed(2)}`;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/* ---------- toasts ---------- */
function toast(msg, kind = "") {
  const wrap = document.getElementById("toasts");
  const el = document.createElement("div");
  el.className = `toast ${kind ? `toast--${kind}` : ""}`;
  el.textContent = msg;
  wrap.appendChild(el);
  setTimeout(() => {
    el.classList.add("toast--leaving");
    setTimeout(() => el.remove(), 320);
  }, 3600);
}

/* ============================================================
   App
   ============================================================ */
const LS_KEY_CART = "gn_cart";

const App = {
  cart: {}, // productId -> qty, persisted so it survives page navigation
  filter: "All",

  loadCart() {
    try {
      this.cart = JSON.parse(localStorage.getItem(LS_KEY_CART)) || {};
    } catch {
      this.cart = {};
    }
  },
  saveCart() {
    localStorage.setItem(LS_KEY_CART, JSON.stringify(this.cart));
  },

  /* ---------- catalog ---------- */

  renderFilters() {
    const el = document.getElementById("filters");
    if (!el) return;
    const cats = ["All", ...new Set(PRODUCTS.map((p) => p.category))];
    el.innerHTML = cats
      .map(
        (c) =>
          `<button class="filter-chip ${c === this.filter ? "active" : ""}" data-cat="${c}">${c}</button>`
      )
      .join("");
    el.querySelectorAll(".filter-chip").forEach((btn) =>
      btn.addEventListener("click", () => {
        this.filter = btn.dataset.cat;
        this.renderFilters();
        this.renderGrid();
      })
    );
  },

  productCard(p, i) {
    return `
      <article class="card" style="animation-delay:${i * 0.06}s">
        <div class="card__art">
          ${p.art}
          ${p.badge ? `<span class="card__badge">${p.badge}</span>` : ""}
        </div>
        <div class="card__body">
          <div class="card__cat">${p.category}</div>
          <h3 class="card__name">${p.name}</h3>
          <p class="card__desc">${p.desc}</p>
          <div class="card__foot">
            <div class="card__price">${fmt(p.price)} <small>USD</small></div>
            <div class="card__actions">
              <button class="btn btn--ghost btn--sm" data-add="${p.id}">Add to pack</button>
              <button class="btn btn--primary btn--sm" data-buy="${p.id}">Buy now</button>
            </div>
          </div>
        </div>
      </article>`;
  },

  bindAddButtons(root) {
    root.querySelectorAll("[data-add]").forEach((btn) =>
      btn.addEventListener("click", () => this.addToCart(btn.dataset.add))
    );
    root.querySelectorAll("[data-buy]").forEach((btn) =>
      btn.addEventListener("click", () => this.buyNow(btn.dataset.buy))
    );
  },

  /** One-click purchase: add the item and go straight to checkout. */
  buyNow(id) {
    this.cart[id] = (this.cart[id] || 0) + 1;
    this.saveCart();
    this.renderCart();
    this.openCheckout();
  },

  /** Jump straight into checkout with a guaranteed non-empty cart. */
  quickCheckout() {
    this.ensureCart();
    this.openCheckout();
  },

  renderGrid() {
    const grid = document.getElementById("product-grid");
    if (!grid) return;
    const items =
      this.filter === "All" ? PRODUCTS : PRODUCTS.filter((p) => p.category === this.filter);
    grid.innerHTML = items.map((p, i) => this.productCard(p, i)).join("");
    this.bindAddButtons(grid);
    const count = document.getElementById("shop-count");
    if (count) count.textContent = `${items.length} product${items.length === 1 ? "" : "s"}`;
  },

  renderFeatured() {
    const grid = document.getElementById("featured-grid");
    if (!grid) return;
    const picks = PRODUCTS.filter((p) => p.badge).slice(0, 3);
    grid.innerHTML = picks.map((p, i) => this.productCard(p, i)).join("");
    this.bindAddButtons(grid);
  },

  /* ---------- cart ---------- */

  addToCart(id) {
    this.cart[id] = (this.cart[id] || 0) + 1;
    this.saveCart();
    this.renderCart();
    const badge = document.getElementById("cart-badge");
    badge.classList.remove("pop");
    void badge.offsetWidth; // restart animation
    badge.classList.add("pop");
    const p = PRODUCTS.find((x) => x.id === id);
    toast(`Added ${p.name} to your pack 🎒`);
  },

  changeQty(id, delta) {
    this.cart[id] = (this.cart[id] || 0) + delta;
    if (this.cart[id] <= 0) delete this.cart[id];
    this.saveCart();
    this.renderCart();
  },

  cartEntries() {
    return Object.entries(this.cart).map(([id, qty]) => ({
      product: PRODUCTS.find((p) => p.id === id),
      qty,
    }));
  },

  cartTotal() {
    return this.cartEntries().reduce((sum, e) => sum + e.product.price * e.qty, 0);
  },

  cartCount() {
    return Object.values(this.cart).reduce((a, b) => a + b, 0);
  },

  renderCart() {
    const count = this.cartCount();
    const badge = document.getElementById("cart-badge");
    badge.hidden = count === 0;
    badge.textContent = count;
    document.getElementById("cart-count-label").textContent = count ? `· ${count} items` : "";

    const body = document.getElementById("cart-items");
    const foot = document.getElementById("cart-foot");
    const entries = this.cartEntries();

    if (!entries.length) {
      body.innerHTML = `<div class="cart-empty"><span>🎒</span>Your pack is empty.<br>The trail awaits.</div>`;
      foot.style.display = "none";
      return;
    }
    foot.style.display = "";
    body.innerHTML = entries
      .map(
        (e) => `
      <div class="cart-item">
        <div class="cart-item__art">${e.product.art}</div>
        <div>
          <div class="cart-item__name">${e.product.name}</div>
          <div class="cart-item__price">${fmt(e.product.price)} each</div>
          <div class="cart-item__qty">
            <button class="qty-btn" data-dec="${e.product.id}">−</button>
            <span>${e.qty}</span>
            <button class="qty-btn" data-inc="${e.product.id}">+</button>
          </div>
        </div>
        <div class="cart-item__total">${fmt(e.product.price * e.qty)}</div>
      </div>`
      )
      .join("");
    body.querySelectorAll("[data-dec]").forEach((b) =>
      b.addEventListener("click", () => this.changeQty(b.dataset.dec, -1))
    );
    body.querySelectorAll("[data-inc]").forEach((b) =>
      b.addEventListener("click", () => this.changeQty(b.dataset.inc, 1))
    );

    const total = this.cartTotal();
    document.getElementById("cart-subtotal").textContent = fmt(total);
    document.getElementById("cart-total").textContent = fmt(total);
    document.getElementById("trees-count").textContent = 3 * count;

    // keep the inline checkout's order summary in step with the cart
    const stepForm = document.getElementById("step-form");
    if (document.getElementById("inline-checkout") && count && stepForm && !stepForm.hidden) {
      this.refreshSummary();
    }
  },

  openCart() {
    document.getElementById("cart-drawer").classList.add("open");
    document.getElementById("cart-overlay").classList.add("show");
  },
  closeCart() {
    document.getElementById("cart-drawer").classList.remove("open");
    document.getElementById("cart-overlay").classList.remove("show");
  },

  /* ---------- checkout ---------- */

  orderId: null,

  newOrderId() {
    const n = Math.floor(100000 + Math.random() * 900000);
    return `GN-${n}`;
  },

  /** Fill the checkout UI (order id, summary, pay amount) from the cart. */
  prepareOrder() {
    this.orderId = this.newOrderId();
    document.getElementById("order-id-label").textContent = this.orderId;
    document.getElementById("success-order-id").textContent = this.orderId;
    this.refreshSummary();
  },

  refreshSummary() {
    const summary = document.getElementById("checkout-summary");
    summary.innerHTML =
      this.cartEntries()
        .map((e) => `<div><span>${e.qty}× ${e.product.name}</span><span>${fmt(e.product.price * e.qty)}</span></div>`)
        .join("") + `<div><span>Total</span><span>${fmt(this.cartTotal())}</span></div>`;
    document.getElementById("pay-amount").textContent = fmt(this.cartTotal());
  },

  /** Make sure there's something to buy — grabs the bestseller if empty. */
  ensureCart() {
    if (!this.cartCount()) {
      const pick = PRODUCTS.find((p) => p.badge === "Bestseller") || PRODUCTS[0];
      this.cart[pick.id] = 1;
      this.saveCart();
      this.renderCart();
    }
  },

  openCheckout() {
    if (!this.cartCount()) return;
    this.closeCart();
    this.prepareOrder();
    this.showStep("form");
    const modal = document.getElementById("checkout-modal");
    if (modal) {
      modal.hidden = false;
    } else {
      document.getElementById("inline-checkout").scrollIntoView({ behavior: "smooth", block: "center" });
    }
  },

  closeCheckout() {
    const modal = document.getElementById("checkout-modal");
    if (modal) {
      modal.hidden = true;
      return;
    }
    // inline checkout never closes — reset to a fresh order form
    this.ensureCart();
    this.prepareOrder();
    this.showStep("form");
  },

  showStep(name) {
    ["form", "processing", "success", "failure"].forEach((s) => {
      document.getElementById(`step-${s}`).hidden = s !== name;
    });
  },

  /* The simulated order pipeline. Each stage takes a realistic
     amount of time; the active chaos scenario decides whether a
     stage fails — and a failure pages the on-call via PagerDuty. */
  async runCheckout() {
    this.showStep("processing");
    const scenario = Chaos.current();
    const slow = scenario.slowFactor || 1;
    const steps = ["validate", "inventory", "payment", "order", "confirm"];
    const els = {};
    steps.forEach((s) => {
      const el = document.querySelector(`.pipeline__step[data-step="${s}"]`);
      el.className = "pipeline__step";
      el.querySelector(".pipeline__time").textContent = "";
      els[s] = el;
    });

    const baseTimes = { validate: 420, inventory: 780, payment: 1500, order: 650, confirm: 500 };

    for (const step of steps) {
      els[step].classList.add("active");
      const t0 = performance.now();
      let duration = baseTimes[step] * slow * (0.8 + Math.random() * 0.5);

      // a failing stage hangs noticeably longer before erroring
      const willFail = scenario.failStep === step;
      if (willFail) duration = Math.max(duration, 2600);
      await sleep(duration);

      const elapsed = ((performance.now() - t0) / 1000).toFixed(2);
      els[step].querySelector(".pipeline__time").textContent = `${elapsed}s`;
      els[step].classList.remove("active");

      if (willFail) {
        els[step].classList.add("failed");
        await sleep(600);
        return this.failCheckout(scenario);
      }
      els[step].classList.add("done");
    }

    await sleep(350);
    this.cart = {};
    this.saveCart();
    this.renderCart();
    this.showStep("success");
  },

  orderContext() {
    return {
      orderId: this.orderId,
      cartValue: this.cartTotal(),
      email: document.getElementById("cust-email").value,
      itemCount: this.cartCount(),
    };
  },

  async failCheckout(scenario) {
    document.getElementById("failure-title").textContent = "Checkout failed";
    document.getElementById("failure-message").textContent = scenario.userMessage;
    document.getElementById("failure-meta").textContent =
      `order_id: ${this.orderId}\n` +
      `error: ${scenario.errorCode}\n` +
      `component: ${scenario.component}\n` +
      `trace_id: ${(crypto.randomUUID?.() ?? [...Array(18)].map(() => Math.random().toString(36)[2]).join('')).slice(0, 18)}`;

    const pdNote = document.getElementById("failure-pd-note");
    pdNote.hidden = true;
    this.showStep("failure");

    const result = await Chaos.triggerIncident(Chaos.scenario, this.orderContext());
    pdNote.hidden = false;
    if (result.ok) {
      pdNote.textContent = "📟 Incident dispatched to the on-call engineer via PagerDuty";
    } else {
      pdNote.textContent = `⚠️ PagerDuty alert NOT sent — ${result.reason}`;
    }
  },

  /* ---------- misc ---------- */

  animateCounters() {
    const els = document.querySelectorAll("[data-count]");
    const io = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (!entry.isIntersecting) return;
          io.unobserve(entry.target);
          const target = +entry.target.dataset.count;
          const t0 = performance.now();
          const dur = 1400;
          const tick = (now) => {
            const p = Math.min((now - t0) / dur, 1);
            const eased = 1 - Math.pow(1 - p, 3);
            entry.target.textContent = Math.round(target * eased).toLocaleString();
            if (p < 1) requestAnimationFrame(tick);
          };
          requestAnimationFrame(tick);
        });
      },
      { threshold: 0.4 }
    );
    els.forEach((el) => io.observe(el));
  },

  init() {
    this.loadCart();
    this.renderFilters();
    this.renderGrid();
    this.renderFeatured();
    this.renderCart();
    this.animateCounters();

    document.querySelectorAll("[data-quick-checkout]").forEach((btn) =>
      btn.addEventListener("click", () => this.quickCheckout())
    );

    document.getElementById("cart-btn").addEventListener("click", () => this.openCart());
    document.getElementById("cart-close").addEventListener("click", () => this.closeCart());
    document.getElementById("cart-overlay").addEventListener("click", () => this.closeCart());
    document.getElementById("checkout-btn").addEventListener("click", () => this.openCheckout());

    const checkoutClose = document.getElementById("checkout-close");
    if (checkoutClose) checkoutClose.addEventListener("click", () => this.closeCheckout());
    document.getElementById("checkout-form").addEventListener("submit", (e) => {
      e.preventDefault();
      this.runCheckout();
    });
    document.getElementById("success-done").addEventListener("click", () => this.closeCheckout());
    document.getElementById("failure-retry").addEventListener("click", () => this.runCheckout());
    document.getElementById("failure-close").addEventListener("click", () => {
      this.closeCheckout();
      this.openCart();
    });

    Chaos.init();

    // The front page embeds checkout in the hero — prime it on load
    // so the form is live the moment the page renders.
    if (document.getElementById("inline-checkout")) {
      this.ensureCart();
      this.prepareOrder();
      this.showStep("form");
    }
  },
};

document.addEventListener("DOMContentLoaded", () => App.init());
