"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Plus, Trash2 } from "lucide-react";
import { useAuthStore } from "@/lib/store";

interface Project {
  id: string;
  name: string;
  description: string;
  createdAt: string;
}

interface Props {
  initialProjects: Project[];
}

export function ProjectList({ initialProjects }: Props) {
  const router = useRouter();
  const role = useAuthStore((s) => s.role);
  const canEdit = role === "admin" || role === "owner";

  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);

  async function handleCreate(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setLoading(true);
    const fd = new FormData(e.currentTarget);
    try {
      const res = await fetch("/api/v1/projects", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: fd.get("name"),
          description: fd.get("description") || undefined,
        }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast.error(data.error ?? "Failed to create project");
        return;
      }
      toast.success("Project created");
      setOpen(false);
      router.refresh();
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(id: string, name: string) {
    if (!confirm(`Delete project "${name}"?`)) return;
    const res = await fetch(`/api/v1/projects/${id}`, { method: "DELETE" });
    if (res.ok) {
      toast.success("Project deleted");
      router.refresh();
    } else {
      toast.error("Failed to delete project");
    }
  }

  return (
    <div>
      {canEdit && (
        <div className="flex justify-end mb-4">
          <Dialog open={open} onOpenChange={setOpen}>
            <DialogTrigger asChild>
              <Button>
                <Plus className="mr-2 h-4 w-4" />
                New project
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create project</DialogTitle>
              </DialogHeader>
              <form onSubmit={handleCreate} className="flex flex-col gap-4 pt-2">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="name">Name</Label>
                  <Input id="name" name="name" required />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="description">Description (optional)</Label>
                  <Input id="description" name="description" />
                </div>
                <Button type="submit" disabled={loading}>
                  {loading ? "Creating…" : "Create"}
                </Button>
              </form>
            </DialogContent>
          </Dialog>
        </div>
      )}
      {initialProjects.length === 0 ? (
        <p className="text-muted-foreground text-center py-12">No projects yet.</p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {initialProjects.map((p) => (
            <Card key={p.id}>
              <CardHeader className="pb-2">
                <div className="flex items-start justify-between gap-2">
                  <CardTitle className="text-base">{p.name}</CardTitle>
                  {canEdit && (
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 shrink-0 text-muted-foreground hover:text-destructive"
                      onClick={() => handleDelete(p.id, p.name)}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  )}
                </div>
                {p.description && (
                  <CardDescription>{p.description}</CardDescription>
                )}
              </CardHeader>
              <CardContent className="text-xs text-muted-foreground">
                {new Date(p.createdAt).toLocaleDateString()}
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
