import { createBrowserRouter, Navigate } from "react-router-dom";
import { AppProviders } from "./AppProviders";
import { AppShell } from "./AppShell";
import { ProtectedLayout } from "./ProtectedLayout";

function Root() {
  return (
    <AppProviders>
      <ProtectedLayout />
    </AppProviders>
  );
}

export const router = createBrowserRouter([
  {
    path: "/",
    element: <Root />,
    children: [
      // Sign-in and setup render without the shell — there is no session yet.
      {
        path: "sign-in",
        lazy: async () => {
          const m = await import("@/pages/SignIn");
          return { Component: m.default };
        },
      },
      {
        path: "setup",
        lazy: async () => {
          const m = await import("@/pages/Setup");
          return { Component: m.default };
        },
      },
      {
        element: <AppShell />,
        children: [
          {
            index: true,
            lazy: async () => {
              const m = await import("@/pages/Dashboard");
              return { Component: m.default };
            },
          },
          {
            path: "projects/new",
            lazy: async () => {
              const m = await import("@/pages/NewProject");
              return { Component: m.default };
            },
          },
          {
            path: "projects/:id",
            lazy: async () => {
              const m = await import("@/pages/ProjectDetail");
              return { Component: m.default };
            },
          },
          {
            path: "logs",
            lazy: async () => {
              const m = await import("@/pages/Logs");
              return { Component: m.default };
            },
          },
          {
            path: "terminal",
            lazy: async () => {
              const m = await import("@/pages/Terminal");
              return { Component: m.default };
            },
          },
          {
            path: "system",
            lazy: async () => {
              const m = await import("@/pages/System");
              return { Component: m.default };
            },
          },
          {
            path: "ports",
            lazy: async () => {
              const m = await import("@/pages/Ports");
              return { Component: m.default };
            },
          },
          {
            path: "databases",
            lazy: async () => {
              const m = await import("@/pages/Databases");
              return { Component: m.default };
            },
          },
          {
            path: "databases/new",
            lazy: async () => {
              const m = await import("@/pages/NewDatabase");
              return { Component: m.default };
            },
          },
          {
            path: "settings",
            lazy: async () => {
              const m = await import("@/pages/Settings");
              return { Component: m.default };
            },
          },
          {
            path: "docs",
            lazy: async () => {
              const m = await import("@/pages/docs/DocsLayout");
              return { Component: m.default };
            },
            children: [
              {
                index: true,
                lazy: async () => {
                  const m = await import("@/pages/docs/DocsIndex");
                  return { Component: m.default };
                },
              },
              {
                path: ":slug",
                lazy: async () => {
                  const m = await import("@/pages/docs/DocPage");
                  return { Component: m.default };
                },
              },
            ],
          },
        ],
      },
      { path: "*", element: <Navigate to="/" replace /> },
    ],
  },
]);
