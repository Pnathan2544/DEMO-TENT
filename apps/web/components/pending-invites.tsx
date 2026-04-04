"use client";

import { Badge } from "@/components/ui/badge";
import { Mail } from "lucide-react";

interface Invite {
  id: string;
  tenantId: string;
  tenantName: string;
  role: string;
  inviterName: string;
  expiresAt: string;
}

interface Props {
  invites: Invite[];
}

export function PendingInvites({ invites }: Props) {
  return (
    <div className="rounded-lg border bg-muted/40 p-4 flex flex-col gap-3">
      <div className="flex items-center gap-2 text-sm font-medium">
        <Mail className="h-4 w-4" />
        You have {invites.length} pending invitation{invites.length > 1 ? "s" : ""}
      </div>
      {invites.map((inv) => (
        <div
          key={inv.id}
          className="flex items-center justify-between gap-4 rounded-md bg-background border px-4 py-3"
        >
          <div className="flex flex-col gap-0.5">
            <span className="font-medium">{inv.tenantName}</span>
            <span className="text-sm text-muted-foreground">
              Invited by {inv.inviterName} ·{" "}
              <Badge variant="outline" className="capitalize text-xs">
                {inv.role}
              </Badge>
              {" · "}
              expires {new Date(inv.expiresAt).toLocaleDateString()}
            </span>
          </div>
          <p className="text-sm text-muted-foreground shrink-0">
            Check your email for the link
          </p>
        </div>
      ))}
    </div>
  );
}
