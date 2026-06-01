import { Suspense } from "react";

import { AuthPageContent } from "./_components/auth-page-content";

export default function AuthPage() {
  return (
    <Suspense>
      <AuthPageContent />
    </Suspense>
  );
}
