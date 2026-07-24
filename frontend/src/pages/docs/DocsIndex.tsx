import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

export default function DocsPage() {
  const navigate = useNavigate();

  useEffect(() => {
    navigate("/docs/introduction", { replace: true });
  }, [navigate]);

  return (
    <div className="flex items-center justify-center min-h-[50vh]">
      <div className="animate-pulse text-muted-foreground font-medium">
        Loading documentation...
      </div>
    </div>
  );
}
