"use client";

import { use, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CheckCircle } from "lucide-react";

export default function InviteAcceptPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token } = use(searchParams);
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!token) {
    return (
      <Card className="w-full max-w-sm">
        <CardContent className="pt-6">
          <p className="text-destructive text-sm">Invalid invite link — no token provided.</p>
        </CardContent>
      </Card>
    );
  }

  async function handleAccept() {
    setLoading(true);
    try {
      const res = await fetch("/api/v1/invites/accept", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token }),
      });
      if (res.status === 401) {
        router.push(`/login?next=/invites/accept?token=${token}`);
        return;
      }
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        const msg =
          res.status === 410
            ? "This invite has expired."
            : res.status === 403
            ? "This invite was sent to a different email address."
            : res.status === 409
            ? "You are already a member of this workspace."
            : data.error ?? "Failed to accept invite";
        setError(msg);
        return;
      }
      const { tenantName } = await res.json();
      toast.success(`Joined ${tenantName}`);
      setAccepted(true);
      setTimeout(() => router.push("/dashboard"), 1500);
    } finally {
      setLoading(false);
    }
  }

  if (accepted) {
    return (
      <Card className="w-full max-w-sm text-center">
        <CardContent className="pt-8 pb-6 flex flex-col items-center gap-3">
          <CheckCircle className="h-10 w-10 text-green-500" />
          <p className="font-medium">Invite accepted! Redirecting…</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="w-full max-w-sm">
      <CardHeader>
        <CardTitle>Team invitation</CardTitle>
        <CardDescription>You&apos;ve been invited to join a workspace.</CardDescription>
      </CardHeader>
      <CardContent>
        {error && (
          <p className="text-sm text-destructive bg-destructive/10 rounded-md p-3">
            {error}
          </p>
        )}
      </CardContent>
      <CardFooter className="flex flex-col gap-3">
        {!error && (
          <Button className="w-full" onClick={handleAccept} disabled={loading}>
            {loading ? "Accepting…" : "Accept invite"}
          </Button>
        )}
        <Link href="/login" className="text-sm text-muted-foreground underline">
          Sign in with a different account
        </Link>
      </CardFooter>
    </Card>
  );
}
