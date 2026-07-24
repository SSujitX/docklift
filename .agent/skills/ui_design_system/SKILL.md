---
name: UI Design System
description: Guide to the UI/CSS architecture, including Tailwind, Shadcn UI, and Theming.
---

# UI Design System Guide

Docklift uses a modern UI stack built on standards-compliant technologies.

## Tech Stack

-   **Framework**: Tailwind CSS (v3 with `tailwindcss-animate` plugin)
-   **Component Library**: Shadcn UI (Headless Radix UI + Tailwind)
-   **Icons**: Lucide React
-   **Theming**: Custom `ThemeProvider` (`frontend/src/lib/theme.tsx`)
-   **Fonts**: Inter (Google Fonts, imported in `src/styles/globals.css`)

## Tailwind Configuration

Found in `frontend/tailwind.config.ts`.
It uses CSS variables for colors to support dynamic theming (dark/light mode).

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
These are **not** an installed library but copy-pasted code you own.

### Core Components
-   `button.tsx`: Variants (default, destructive, outline, secondary, ghost, link).
-   `card.tsx`: Structure for widgets (Header, Title, Content).
-   `dialog.tsx`: Modals.
-   `input.tsx` / `textarea.tsx`: Form elements.
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

-   **"Styles missing"**: ensure the file path is included in `tailwind.config.ts` content array.
-   **"Dark mode not working"**: Ensure `ThemeProvider` wraps the app in `src/app/AppProviders.tsx`.
