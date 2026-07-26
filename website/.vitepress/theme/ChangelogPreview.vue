<script setup lang="ts">
import { computed, ref } from "vue";
import { data as releases } from "../changelog.data";

const PAGE_SIZE = 3;
const visible = ref(PAGE_SIZE);

const shown = computed(() => releases.slice(0, visible.value));
const hasMore = computed(() => visible.value < releases.length);
const remaining = computed(() => Math.max(0, releases.length - visible.value));

function loadMore() {
  visible.value = Math.min(visible.value + PAGE_SIZE, releases.length);
}
</script>

<template>
  <section class="dl-changelog" aria-labelledby="dl-changelog-heading">
    <div class="dl-changelog__header">
      <h2 id="dl-changelog-heading">Changelog</h2>
      <a class="dl-changelog__all" href="/changelog">View full changelog →</a>
    </div>
    <p class="dl-changelog__lead">
      Latest releases from the repository
      <code>CHANGELOG.md</code> (updated automatically on each release).
    </p>

    <div v-if="releases.length === 0" class="dl-changelog__empty">
      No release notes found yet.
    </div>

    <div v-else class="dl-changelog__panel">
      <div class="dl-changelog__scroll vp-doc">
        <article
          v-for="(release, i) in shown"
          :key="release.version || i"
          class="dl-changelog__release"
          v-html="release.html"
        />
      </div>

      <div class="dl-changelog__actions">
        <button
          v-if="hasMore"
          type="button"
          class="dl-changelog__more"
          @click="loadMore"
        >
          Load more
          <span class="dl-changelog__more-count">({{ remaining }} left)</span>
        </button>
        <a v-else class="dl-changelog__all" href="/changelog">
          Open full changelog →
        </a>
      </div>
    </div>
  </section>
</template>
