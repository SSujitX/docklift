import DefaultTheme from "vitepress/theme";
import type { Theme } from "vitepress";
import ChangelogPreview from "./ChangelogPreview.vue";
import "./custom.css";

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component("ChangelogPreview", ChangelogPreview);
  },
} satisfies Theme;
