<script setup lang="ts">
import { ref } from "vue";
import ChangelogPreview from "./ChangelogPreview.vue";

const INSTALL =
  "curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash";

const copied = ref(false);

async function copyInstall() {
  try {
    await navigator.clipboard.writeText(INSTALL);
    copied.value = true;
    window.setTimeout(() => (copied.value = false), 1500);
  } catch {
    copied.value = false;
  }
}

const features = [
  {
    title: "One-command install",
    text: "A curl install on Ubuntu or Debian. The dashboard comes up on port 8080 and asks for a one-time setup code.",
    link: "/guide/installation",
    go: "Installation",
  },
  {
    title: "Git to deploy",
    text: "Connect the GitHub App, build with a Dockerfile or Railpack, run several services in one project, deploy on every push.",
    link: "/guide/deployment",
    go: "Deployment",
  },
  {
    title: "Domains and HTTPS",
    text: "Point a domain at a service and Let's Encrypt issues the certificate. Host ports stay opt-in.",
    link: "/guide/domains",
    go: "Custom domains",
  },
  {
    title: "Managed databases",
    text: "Postgres, MySQL, MariaDB, Redis and Mongo. Private by default, linked into the apps that need them.",
    link: "/guide/databases",
    go: "Databases",
  },
  {
    title: "Ops toolbox",
    text: "Host stats, a web terminal, logs, backup and restore, and a reference of the commands you will need.",
    link: "/guide/commands",
    go: "Commands",
  },
  {
    title: "API and MCP",
    text: "Everything in the dashboard is also a REST endpoint, and AI agents can drive it over MCP with a scoped token.",
    link: "/guide/mcp",
    go: "AI agents",
  },
];
</script>

<template>
  <div class="dl-home">
    <section class="dl-hero">
      <div class="dl-hero__text">
        <h1>Deploy Docker apps on your own server</h1>
        <p class="dl-hero__lead">
          Docklift is an open-source dashboard for a single VPS. Push to GitHub
          or upload a ZIP, and it builds the image, runs it, routes your domain
          and sets up HTTPS.
        </p>
      </div>

      <div class="dl-install">
        <pre><span class="dl-prompt">$ </span>{{ INSTALL }}</pre>
        <button type="button" @click="copyInstall">
          {{ copied ? "Copied" : "Copy" }}
        </button>
      </div>
      <p class="dl-hero__note">
        Then open <code>http://SERVER_IP:8080</code>, paste the setup code the
        installer prints, and create your admin account.
      </p>

      <div class="dl-actions">
        <a class="dl-btn dl-btn--primary" href="/guide/quick-start">Quick start</a>
        <a class="dl-btn" href="/guide/commands">Useful commands</a>
        <a
          class="dl-btn"
          href="https://github.com/SSujitX/docklift"
          target="_blank"
          rel="noreferrer"
          >GitHub</a
        >
      </div>

      <figure class="dl-shot">
        <img
          class="dl-shot__light"
          src="/screens/projects-light.png"
          width="1440"
          height="900"
          alt="The Docklift dashboard: a list of projects with status, branch, domain and CPU and memory use."
        />
        <img
          class="dl-shot__dark"
          src="/screens/projects-dark.png"
          width="1440"
          height="900"
          alt="The Docklift dashboard: a list of projects with status, branch, domain and CPU and memory use."
        />
      </figure>
    </section>

    <section class="dl-section" aria-labelledby="dl-what">
      <h2 id="dl-what">What it does</h2>
      <ul class="dl-list">
        <li v-for="f in features" :key="f.title">
          <a :href="f.link">
            <span class="dl-list__title">{{ f.title }}</span>
            <span class="dl-list__text">{{ f.text }}</span>
            <span class="dl-list__go">{{ f.go }} →</span>
          </a>
        </li>
      </ul>
    </section>

    <ChangelogPreview />
    <div class="dl-home__end" />
  </div>
</template>
