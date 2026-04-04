"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { WorkspaceSwitcher } from "@/components/workspace-switcher";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  LayoutDashboard,
  Users,
  ClipboardList,
  CreditCard,
  LogOut,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/lib/store";

const navItems = [
  { href: "/dashboard", label: "Dashboard", icon: LayoutDashboard, minRole: "viewer" },
  { href: "/members", label: "Members", icon: Users, minRole: "viewer" },
  { href: "/audit", label: "Audit Log", icon: ClipboardList, minRole: "admin" },
  { href: "/billing", label: "Billing", icon: CreditCard, minRole: "viewer" },
];

const roleWeight: Record<string, number> = {
  owner: 4,
  admin: 3,
  member: 2,
  viewer: 1,
};

interface Props {
  role: string;
}

export function NavSidebar({ role }: Props) {
  const pathname = usePathname();
  const router = useRouter();
  const clear = useAuthStore((s) => s.clear);

  async function handleLogout() {
    await fetch("/api/v1/auth/logout", { method: "POST" });
    clear();
    router.push("/login");
  }

  const visible = navItems.filter(
    (item) => roleWeight[role] >= roleWeight[item.minRole]
  );

  return (
    <aside className="w-56 shrink-0 flex flex-col border-r bg-muted/40 p-3 gap-1">
      <div className="mb-2">
        <WorkspaceSwitcher />
      </div>
      <Separator className="mb-2" />
      {visible.map((item) => {
        const Icon = item.icon;
        const active = pathname.startsWith(item.href);
        return (
          <Link key={item.href} href={item.href}>
            <Button
              variant={active ? "secondary" : "ghost"}
              className={cn("w-full justify-start gap-2", active && "font-semibold")}
            >
              <Icon className="h-4 w-4" />
              {item.label}
            </Button>
          </Link>
        );
      })}
      <div className="mt-auto">
        <Separator className="mb-2" />
        <Button
          variant="ghost"
          className="w-full justify-start gap-2 text-muted-foreground"
          onClick={handleLogout}
        >
          <LogOut className="h-4 w-4" />
          Log out
        </Button>
      </div>
    </aside>
  );
}
