import { defineConfig } from "vitepress";

const guide = [
  { text: "Quick Start", link: "/guide/quick-start" },
  { text: "Introduction", link: "/guide/introduction" },
  { text: "Installation", link: "/guide/installation" },
  { text: "Useful Commands", link: "/guide/commands" },
  { text: "Changelog", link: "/changelog" },
];

const deploy = [
  { text: "GitHub Integration", link: "/guide/github" },
  { text: "Auto-Deploy", link: "/guide/autodeploy" },
  { text: "Deployment", link: "/guide/deployment" },
  { text: "Dockerfile & Railpack", link: "/guide/dockerfile" },
  { text: "Environment Variables", link: "/guide/environment" },
  { text: "Managed Databases", link: "/guide/databases" },
  { text: "Custom Domains", link: "/guide/domains" },
  { text: "Port Management", link: "/guide/ports" },
  { text: "File Management", link: "/guide/files" },
];

const operate = [
  { text: "System Overview", link: "/guide/system" },
  { text: "Web Terminal", link: "/guide/terminal" },
  { text: "Backup & Restore", link: "/guide/backup" },
  { text: "Profile", link: "/guide/profile" },
  { text: "Reset Password", link: "/guide/reset-password" },
  { text: "API Reference", link: "/guide/api" },
  { text: "Troubleshooting", link: "/guide/troubleshooting" },
];

export default defineConfig({
  title: "Docklift",
  description:
    "Self-hosted PaaS for Docker on your VPS — docs, quick start, and commands.",
  cleanUrls: true,
  lastUpdated: true,
  ignoreDeadLinks: true,
  // Changelog commit text is untrusted enough that raw HTML must not render site-wide.
  markdown: { html: false },
  head: [
    ["link", { rel: "icon", href: "/favicon.svg" }],
    ["meta", { name: "theme-color", content: "#0891b2" }],
    [
      "meta",
      {
        property: "og:title",
        content: "Docklift Docs — Self-hosted Docker PaaS",
      },
    ],
    [
      "meta",
      {
        property: "og:description",
        content:
          "Install, deploy, domains, databases, and ops commands for Docklift.",
      },
    ],
  ],
  themeConfig: {
    logo: { src: "/logo.svg", alt: "Docklift" },
    siteTitle: "Docklift",
    nav: [
      { text: "Quick Start", link: "/guide/quick-start" },
      { text: "Commands", link: "/guide/commands" },
      { text: "Changelog", link: "/changelog" },
      { text: "Install", link: "/guide/installation" },
      {
        text: "GitHub",
        link: "https://github.com/SSujitX/docklift",
      },
    ],
    sidebar: [
      { text: "Get started", items: guide },
      { text: "Deploy", items: deploy },
      { text: "Operate", items: operate },
    ],
    socialLinks: [
      { icon: "github", link: "https://github.com/SSujitX/docklift" },
    ],
    search: { provider: "local" },
    editLink: {
      pattern: "https://github.com/SSujitX/docklift/edit/master/website/:path",
      text: "Edit this page",
    },
    footer: {
      message: "Open-source self-hosted PaaS for Docker",
      copyright: "© Docklift contributors",
    },
    outline: { level: [2, 3] },
  },
});
