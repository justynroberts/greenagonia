/* ============================================================
   Greenagonia — product catalog
   Product art is generated SVG so the site has zero external
   image dependencies and always renders.
   ============================================================ */

// deterministic, seeded per-product palette helpers
function art(id, hue1, hue2, icon) {
  return `
  <svg viewBox="0 0 400 340" xmlns="http://www.w3.org/2000/svg" role="img">
    <defs>
      <linearGradient id="bg-${id}" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" stop-color="${hue1}"/>
        <stop offset="1" stop-color="${hue2}"/>
      </linearGradient>
      <radialGradient id="glow-${id}" cx="0.5" cy="0.35" r="0.7">
        <stop offset="0" stop-color="rgba(255,255,255,0.16)"/>
        <stop offset="1" stop-color="rgba(255,255,255,0)"/>
      </radialGradient>
    </defs>
    <rect width="400" height="340" fill="url(#bg-${id})"/>
    <rect width="400" height="340" fill="url(#glow-${id})"/>
    <path d="M0 340 L0 290 L70 240 L150 300 L230 230 L310 295 L400 245 L400 340 Z"
          fill="rgba(0,0,0,0.18)"/>
    <g transform="translate(200,150)">${icon}</g>
  </svg>`;
}

const ICONS = {
  jacket: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-36)">
    <path d="M22 12 L32 8 L42 12 L54 22 L48 34 L44 30 L44 62 L20 62 L20 30 L16 34 L10 22 Z"/>
    <path d="M32 8 L32 62" stroke-width="4" opacity="0.6"/>
    <path d="M26 12 a6 6 0 0 0 12 0" stroke-width="4"/>
  </g>`,
  pack: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-36)">
    <rect x="16" y="18" width="32" height="44" rx="10"/>
    <path d="M24 18 v-4 a8 8 0 0 1 16 0 v4"/>
    <path d="M16 38 h32" stroke-width="4" opacity="0.6"/>
    <rect x="26" y="44" width="12" height="12" rx="3" stroke-width="4"/>
  </g>`,
  bottle: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-36)">
    <path d="M26 14 h12 M28 14 v6 c-6 4 -8 8 -8 14 v22 a6 6 0 0 0 6 6 h12 a6 6 0 0 0 6 -6 V34 c0 -6 -2 -10 -8 -14 v-6"/>
    <path d="M22 40 h20" stroke-width="4" opacity="0.6"/>
  </g>`,
  tent: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-30)">
    <path d="M32 10 L6 54 H58 Z"/>
    <path d="M32 10 L32 54 M24 54 L32 38 L40 54" stroke-width="4" opacity="0.7"/>
  </g>`,
  boot: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-34)">
    <path d="M18 12 h16 v22 c8 0 14 4 20 10 a8 8 0 0 1 -6 14 H18 Z"/>
    <path d="M18 48 h36" stroke-width="4" opacity="0.6"/>
    <path d="M24 18 h6 M24 26 h6" stroke-width="4" opacity="0.7"/>
  </g>`,
  lantern: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-36)">
    <path d="M24 14 a8 8 0 0 1 16 0"/>
    <rect x="20" y="14" width="24" height="8" rx="3"/>
    <path d="M22 22 L18 48 a8 8 0 0 0 8 8 h12 a8 8 0 0 0 8 -8 L42 22"/>
    <circle cx="32" cy="38" r="6" stroke-width="4" opacity="0.8"/>
  </g>`,
  beanie: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-34)">
    <path d="M12 44 a20 20 0 0 1 40 0"/>
    <rect x="10" y="44" width="44" height="12" rx="5"/>
    <circle cx="32" cy="20" r="5" stroke-width="5"/>
    <path d="M22 40 v-8 M32 38 v-10 M42 40 v-8" stroke-width="4" opacity="0.6"/>
  </g>`,
  flask: `<g stroke="rgba(255,255,255,0.9)" stroke-width="7" fill="none" stroke-linecap="round" stroke-linejoin="round" transform="scale(1.5) translate(-32,-36)">
    <path d="M24 10 h16 v10 c8 3 12 9 12 17 v12 a10 10 0 0 1 -10 10 H22 a10 10 0 0 1 -10 -10 V37 c0 -8 4 -14 12 -17 Z"/>
    <path d="M24 16 h16" stroke-width="4" opacity="0.6"/>
    <path d="M20 42 c4 -3 8 -3 12 0 s8 3 12 0" stroke-width="4" opacity="0.7"/>
  </g>`,
};

const PRODUCTS = [
  {
    id: "gn-shell-01",
    name: "Stormline Recycled Shell",
    category: "Apparel",
    price: 249,
    badge: "Bestseller",
    desc: "3-layer waterproof shell spun from 41 recycled bottles.",
    art: art("p1", "#14532d", "#052e16", ICONS.jacket),
  },
  {
    id: "gn-pack-01",
    name: "Caldera 38L Trail Pack",
    category: "Packs",
    price: 189,
    badge: "New",
    desc: "Hemp-canvas pack with lifetime repair guarantee.",
    art: art("p2", "#1e3a5f", "#0c1f33", ICONS.pack),
  },
  {
    id: "gn-bottle-01",
    name: "Evergreen Steel Bottle",
    category: "Camp",
    price: 39,
    badge: null,
    desc: "1L insulated steel — keeps cold 24h, hot 12h.",
    art: art("p3", "#0f766e", "#042f2e", ICONS.bottle),
  },
  {
    id: "gn-tent-01",
    name: "Sequoia 2P Ultralight Tent",
    category: "Camp",
    price: 429,
    badge: "Limited",
    desc: "1.4kg, recycled ripstop, pitches in 90 seconds.",
    art: art("p4", "#4d7c0f", "#1a2e05", ICONS.tent),
  },
  {
    id: "gn-boot-01",
    name: "Ridgewalker Hiking Boots",
    category: "Footwear",
    price: 219,
    badge: null,
    desc: "Regenerative leather, resoleable, broken-in feel from day one.",
    art: art("p5", "#7c2d12", "#3b1106", ICONS.boot),
  },
  {
    id: "gn-lantern-01",
    name: "Firefly Solar Lantern",
    category: "Camp",
    price: 59,
    badge: "New",
    desc: "200 lumens, charges by sun, doubles as a power bank.",
    art: art("p6", "#a16207", "#422006", ICONS.lantern),
  },
  {
    id: "gn-beanie-01",
    name: "Summit Merino Beanie",
    category: "Apparel",
    price: 34,
    badge: null,
    desc: "Traceable merino from regenerative farms in Greenagonia.",
    art: art("p7", "#6d28d9", "#2e1065", ICONS.beanie),
  },
  {
    id: "gn-flask-01",
    name: "Basecamp Coffee Flask",
    category: "Camp",
    price: 44,
    badge: null,
    desc: "Brews on trail. 100% recycled steel, zero plastic.",
    art: art("p8", "#9f1239", "#4c0519", ICONS.flask),
  },
];
