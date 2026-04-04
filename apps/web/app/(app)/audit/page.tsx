import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { apiFetch } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

interface AuditEvent {
  id: string;
  actorId: string;
  actorName: string;
  targetId: string | null;
  targetName: string | null;
  action: string;
  meta: Record<string, unknown>;
  createdAt: string;
}

export default async function AuditPage() {
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const meRes = await apiFetch("/me", cookieHeader);
  const me = meRes.ok ? await meRes.json() : null;

  if (me?.role !== "admin" && me?.role !== "owner") {
    redirect("/dashboard");
  }

  const auditRes = await apiFetch("/audit", cookieHeader);
  const events: AuditEvent[] = auditRes.ok ? await auditRes.json() : [];

  return (
    <div className="max-w-5xl mx-auto">
      <h1 className="text-2xl font-semibold mb-6">Audit Log</h1>
      {events.length === 0 ? (
        <p className="text-muted-foreground text-center py-12">No events yet.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Details</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {events.map((ev) => (
              <TableRow key={ev.id}>
                <TableCell className="text-muted-foreground text-sm whitespace-nowrap">
                  {new Date(ev.createdAt).toLocaleString()}
                </TableCell>
                <TableCell className="font-medium">{ev.actorName}</TableCell>
                <TableCell>
                  <Badge variant="outline">{ev.action.replace(/_/g, " ")}</Badge>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {Object.entries(ev.meta ?? {})
                    .map(([k, v]) => `${k}: ${v}`)
                    .join(" · ")}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
