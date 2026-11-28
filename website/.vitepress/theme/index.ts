import { h } from "vue";
import DefaultTheme from "vitepress/theme";
import type { Theme } from "vitepress";
import ChangelogPreview from "./ChangelogPreview.vue";
import Home from "./Home.vue";
import "./custom.css";

export default {
  extends: DefaultTheme,
  // The home page is a component in a layout slot: raw HTML is disabled in
  // markdown (see config.ts), so index.md cannot mount it itself.
  Layout: () => h(DefaultTheme.Layout, null, { "home-hero-before": () => h(Home) }),
  enhanceApp({ app }) {
    app.component("ChangelogPreview", ChangelogPreview);
  },
} satisfies Theme;
