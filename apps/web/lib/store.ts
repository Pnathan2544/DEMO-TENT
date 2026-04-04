"use client";

import { create } from "zustand";

export interface User {
  id: string;
  email: string;
  name: string;
}

export interface Tenant {
  id: string;
  name: string;
  slug: string;
}

export interface TenantEntry {
  id: string;
  name: string;
  slug: string;
  role: string;
}

interface AuthStore {
  user: User | null;
  tenant: Tenant | null;
  role: string | null;
  tenants: TenantEntry[];
  setAuth: (
    user: User,
    tenant: Tenant,
    role: string,
    tenants: TenantEntry[]
  ) => void;
  clear: () => void;
}

export const useAuthStore = create<AuthStore>((set) => ({
  user: null,
  tenant: null,
  role: null,
  tenants: [],
  setAuth: (user, tenant, role, tenants) =>
    set({ user, tenant, role, tenants }),
  clear: () => set({ user: null, tenant: null, role: null, tenants: [] }),
}));
