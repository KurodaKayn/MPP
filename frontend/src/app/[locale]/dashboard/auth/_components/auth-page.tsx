import { Suspense } from "react";

import { AuthPageContent } from "./auth-page-content";

export function AuthPage() {
  return (
    <Suspense>
      <AuthPageContent />
    </Suspense>
  );
}
