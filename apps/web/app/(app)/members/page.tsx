import { cookies } from "next/headers";
import { apiFetch } from "@/lib/api";
import { MemberTable } from "@/components/member-table";
import { InviteForm } from "@/components/invite-form";

export default async function MembersPage() {
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const [meRes, membersRes, invitesRes] = await Promise.all([
    apiFetch("/me", cookieHeader),
    apiFetch("/members", cookieHeader),
    apiFetch("/invites", cookieHeader),
  ]);

  const me = meRes.ok ? await meRes.json() : null;
  const members = membersRes.ok ? await membersRes.json() : [];
  const invites = invitesRes.ok ? await invitesRes.json() : [];
  const canManage = me?.role === "admin" || me?.role === "owner";

  return (
    <div className="max-w-3xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-semibold">Members</h1>
        {canManage && <InviteForm />}
      </div>
      <MemberTable
        members={members}
        invites={invites}
        currentUserId={me?.id}
        currentRole={me?.role}
      />
    </div>
  );
}
