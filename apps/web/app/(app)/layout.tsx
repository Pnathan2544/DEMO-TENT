import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { apiFetch } from "@/lib/api";
import { AuthProvider } from "@/components/auth-provider";
import { NavSidebar } from "@/components/nav-sidebar";

export default async function AppLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const cookieStore = await cookies();
  const cookieHeader = cookieStore.toString();

  const [meRes, tenantsRes] = await Promise.all([
    apiFetch("/me", cookieHeader),
    apiFetch("/auth/tenants", cookieHeader),
  ]);

  if (!meRes.ok) redirect("/login");

  const me = await meRes.json();
  const tenants = tenantsRes.ok ? await tenantsRes.json() : [];

  return (
    <AuthProvider
      user={{ id: me.id, email: me.email, name: me.name }}
      tenant={{ id: me.tenantId, name: tenants.find((t: { id: string }) => t.id === me.tenantId)?.name ?? "", slug: tenants.find((t: { id: string }) => t.id === me.tenantId)?.slug ?? "" }}
      role={me.role}
      tenants={tenants}
    >
      <div className="flex h-screen">
        <NavSidebar role={me.role} />
        <main className="flex-1 overflow-auto p-6">{children}</main>
      </div>
    </AuthProvider>
  );
}
