import { cookies } from "next/headers";
import { apiFetch } from "@/lib/api";
import { ProjectList } from "@/components/project-list";
import { PendingInvites } from "@/components/pending-invites";

export default async function DashboardPage() {
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const [projectsRes, invitesRes] = await Promise.all([
    apiFetch("/projects", cookieHeader),
    apiFetch("/me/invites", cookieHeader),
  ]);

  const projects = projectsRes.ok ? await projectsRes.json() : [];
  const pendingInvites = invitesRes.ok ? await invitesRes.json() : [];

  return (
    <div className="max-w-4xl mx-auto flex flex-col gap-6">
      <h1 className="text-2xl font-semibold">Dashboard</h1>
      {pendingInvites.length > 0 && (
        <PendingInvites invites={pendingInvites} />
      )}
      <ProjectList initialProjects={projects} />
    </div>
  );
}
