import { Outlet } from "react-router-dom";

/** Authenticated shell — AuthProvider handles redirects for unauthenticated users. */
export function ProtectedLayout() {
  return <Outlet />;
}
