"use client";

import { useEffect } from "react";
import { useAuthStore, type User, type Tenant, type TenantEntry } from "@/lib/store";

interface Props {
  user: User;
  tenant: Tenant;
  role: string;
  tenants: TenantEntry[];
  children: React.ReactNode;
}

export function AuthProvider({ user, tenant, role, tenants, children }: Props) {
  const setAuth = useAuthStore((s) => s.setAuth);

  useEffect(() => {
    setAuth(user, tenant, role, tenants);
  }, [user, tenant, role, tenants, setAuth]);

  return <>{children}</>;
}
