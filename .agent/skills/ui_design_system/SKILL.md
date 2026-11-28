---
name: UI Design System
description: Guide to the UI/CSS architecture, including Tailwind, Shadcn UI, and Theming.
---

# UI Design System Guide

Docklift uses a modern UI stack built on standards-compliant technologies.

## Tech Stack

-   **Framework**: Tailwind CSS 4 via `@tailwindcss/vite` (Vite 8), with `tw-animate-css` for animation utilities (no `tailwindcss-animate`, no PostCSS config). Production CSS is minified by Lightning CSS (Vite 8 default): values containing `var()` keep their spaces, so the CSS is slightly larger than with esbuild, and it targets Chrome/Edge 111, Firefox 114, Safari 16.4
-   **Component Library**: shadcn-style components (headless Radix UI + Tailwind), copied into the repo
-   **Icons**: Lucide React
-   **Theming**: Custom `ThemeProvider` (`frontend/src/lib/theme.tsx`)
-   **Fonts**: Inter (Google Fonts, imported at the top of `src/styles/globals.css`); mono is the system `ui-monospace` stack

## Tailwind Configuration

There is **no `tailwind.config.ts`**. Tailwind 4 is configured in CSS, in
`frontend/src/styles/globals.css`:

-   `@import 'tailwindcss'` and `@import 'tw-animate-css'`.
-   `@custom-variant dark (&:is(.dark *))`: dark mode is the `.dark` class on the root, set by
    `ThemeProvider`. `@custom-variant hover (&:hover)` keeps `hover:` working on touch devices
    (v4 otherwise gates it on `(hover: hover)`).
-   `@theme { ... }` maps design tokens to utilities: `--color-background: hsl(var(--background))`
    and so on, plus `--font-sans`, `--font-mono` and the text line-heights. A new color token needs
    both the raw variable (in `:root` and `.dark`) and a `--color-*` entry in `@theme`.
-   The `@theme` block also pins the Tailwind 3 values of the palette shades the app uses (amber,
    blue, cyan, emerald, ...), because v4's default oklch colors look brighter. Keep using those
    shades; do not remove the overrides.
-   `@utility space-x-*` / `space-y-*` re-declare the v3 sibling selectors (v4 changed them and hidden
    siblings such as inactive Radix tab panels would leave extra margin).
-   Class detection is automatic (source files are scanned); there is no `content` array.

Colors use CSS variables holding bare HSL triples (`--primary: 0 0% 9%`), so the same utility works
in both themes. The values live in `:root` and `.dark` inside `@layer base`.

### Key Colors (HSL vars)
-   `bg-background` / `text-foreground`: Main page colors.
-   `bg-card` / `text-card-foreground`: Card elements.
-   `bg-primary` / `text-primary-foreground`: Main actions (buttons).
-   `bg-muted` / `text-muted-foreground`: Secondary text/backgrounds.
-   `bg-destructive`: Error states.
-   `bg-brand` / `text-brand`: Accent for active nav, focus and primary CTAs.
-   `bg-sidebar` / `text-sidebar-foreground` / `border-sidebar-border` /
    `bg-sidebar-accent`: The left rail's own surface, one step recessed from
    content so the shell reads as a separate plane in both themes.

Prefer `brand` over hard-coded `cyan-500` in new UI; the token is what keeps the
rail, the active states and the CTAs in sync.

## Shadcn UI Components

Located in `frontend/src/components/ui/`.
These are **not** an installed library but code you own (there is no `components.json`; add new ones by hand).

### Core Components
-   `button.tsx`: Variants (default, destructive, outline, secondary, ghost, link).
-   `card.tsx`: Structure for widgets (Header, Title, Content).
-   `dialog.tsx`: Modals.
-   `input.tsx` / `textarea.tsx`: Form elements.
-   `tabs.tsx`: Tabs (`@radix-ui/react-tabs`).
-   `sonner.tsx`: Toast notifications.

### Customizing Components
To change a component's look, edit the file directly in `src/components/ui/`.
Example: To make all buttons rounded, edit `frontend/src/components/ui/button.tsx`.

## Layouts & Structure

-   **Responsiveness**: Mobile-first approach using standard Tailwind breakpoints (`sm:`, `md:`, `lg:`).
-   **Shell**: `app/AppShell.tsx` owns the fixed left rail, the thin top bar and
    the content `<main>` (max width plus padding). Pages return their content
    directly — no outer container, `min-h-screen`, header or footer.
-   **Page titles**: use `PageHeader` (and `StatChip` for counts) from
    `components/shell/PageHeader.tsx` so every page shares one rhythm.
-   **Rail width**: driven by the `--shell-rail` custom property; `.shell-rail`
    sizes the rail and `.shell-inset` offsets content, so collapsing only has to
    override one variable.
-   **Scroll containment**: long panes (logs, file trees, terminals) scroll
    *inside* their own box with `.shell-scroll`. Never let a child call
    `scrollIntoView()` — it scrolls the whole `<main>` and makes the fixed rail
    and top bar feel like they jump.

## Troubleshooting Styles

-   **"Styles missing"**: Tailwind 4 detects classes by scanning source files; check the class is written as a complete literal string (no string concatenation) and that the dev server was restarted after editing `globals.css`. A new color needs its `--color-*` entry in `@theme`.
-   **Utility renamed or behaves differently**: this app is on Tailwind 4; check the v4 upgrade notes before copying v3 snippets.
-   **"Dark mode not working"**: Ensure `ThemeProvider` wraps the app in `src/app/AppProviders.tsx`.
