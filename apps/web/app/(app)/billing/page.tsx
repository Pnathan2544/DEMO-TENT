"use client";

import { useEffect, useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CreditCard, ExternalLink } from "lucide-react";

interface Billing {
  plan: string;
  status: "active" | "free";
}

export default function BillingPage() {
  const [billing, setBilling] = useState<Billing | null>(null);
  const [loading, setLoading] = useState(true);
  const [portalLoading, setPortalLoading] = useState(false);

  useEffect(() => {
    fetch("/api/v1/me/billing")
      .then((r) => r.json())
      .then(setBilling)
      .catch(() => toast.error("Failed to load billing"))
      .finally(() => setLoading(false));
  }, []);

  async function openPortal() {
    setPortalLoading(true);
    try {
      const res = await fetch("/api/v1/me/billing/portal", { method: "POST" });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast.error(data.error ?? "Billing portal unavailable");
        return;
      }
      const { url } = await res.json();
      window.open(url, "_blank");
    } finally {
      setPortalLoading(false);
    }
  }

  return (
    <div className="max-w-lg mx-auto">
      <h1 className="text-2xl font-semibold mb-6">Billing</h1>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <CreditCard className="h-5 w-5" />
            Current plan
          </CardTitle>
          <CardDescription>Manage your subscription</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {loading ? (
            <p className="text-muted-foreground text-sm">Loading…</p>
          ) : (
            <>
              <div className="flex items-center gap-3">
                <span className="font-medium capitalize">{billing?.plan ?? "—"}</span>
                <Badge variant={billing?.status === "active" ? "default" : "secondary"}>
                  {billing?.status ?? "free"}
                </Badge>
              </div>
              <Button
                onClick={openPortal}
                disabled={portalLoading}
                variant="outline"
                className="w-fit"
              >
                <ExternalLink className="mr-2 h-4 w-4" />
                {portalLoading ? "Opening…" : "Manage subscription"}
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
