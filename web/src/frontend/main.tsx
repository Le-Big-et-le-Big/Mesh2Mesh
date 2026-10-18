import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";

import { ApiError, meQuery } from "@/api";
import { createAppRouter } from "@/router";
import "@/lib/theme";
import "@/styles.css";

function onError(err: unknown) {
  if (err instanceof ApiError && err.status === 401) {
    queryClient.setQueryData(meQuery.queryKey, null);
    void router.navigate({ to: "/login", search: { redirect: router.state.location.href } });
  }
}

const queryClient = new QueryClient({
  queryCache: new QueryCache({ onError }),
  mutationCache: new MutationCache({ onError }),
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
    },
  },
});

const router = createAppRouter(queryClient);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
