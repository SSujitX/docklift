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
-   **Theming**: `next-themes` (Dark/Light mode support)
-   **Fonts**: Geist Sans & Geist Mono (via `next/font`)

## Tailwind Configuration

Found in `frontend/tailwind.config.ts`.
It uses CSS variables for colors to support dynamic theming (dark/light mode).

### Key Colors (HSL vars)
-   `bg-background` / `text-foreground`: Main page colors.
-   `bg-card` / `text-card-foreground`: Card elements.
-   `bg-primary` / `text-primary-foreground`: Main actions (buttons).
-   `bg-muted` / `text-muted-foreground`: Secondary text/backgrounds.
-   `bg-destructive`: Error states.

## Shadcn UI Components

Located in `frontend/components/ui/`.
These are **not** an installed library but copy-pasted code you own.

### Core Components
-   `button.tsx`: Variants (default, destructive, outline, secondary, ghost, link).
-   `card.tsx`: Structure for widgets (Header, Title, Content).
-   `dialog.tsx`: Modals.
-   `input.tsx` / `textarea.tsx`: Form elements.
-   `sonner.tsx`: Toast notifications.

### Customizing Components
To change a component's look, edit the file directly in `components/ui/`.
Example: To make all buttons rounded, edit `frontend/components/ui/button.tsx`.

## Layouts & Structure

-   **Responsiveness**: Mobile-first approach using standard Tailwind breakpoints (`sm:`, `md:`, `lg:`).
-   **Containers**: Use `container mx-auto px-4` for page content wrappers.

## Troubleshooting Styles

-   **"Styles missing"**: ensure the file path is included in `tailwind.config.ts` content array.
-   **"Dark mode not working"**: Ensure `ThemeProvider` wraps the app in `layout.tsx`.
