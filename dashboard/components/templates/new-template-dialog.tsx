"use client";

import { useState } from "react";
import { FileText } from "lucide-react";
import type { Template, TemplateChannel } from "@/lib/api/types";
import { useCreateTemplate } from "@/lib/queries/use-templates";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SimpleSelect } from "@/components/common/simple-select";

// A new template starts from a safe default body using only the wired variables.
function defaultBody(): string {
  return "Your verification code is {{code}}. It expires in {{expiry}}.";
}

export function NewTemplateDialog({
  open,
  onClose,
  onCreate,
}: {
  open: boolean;
  onClose: () => void;
  onCreate: (t: Template) => void;
}) {
  const create = useCreateTemplate();
  const [name, setName] = useState("");
  const [channel, setChannel] = useState<TemplateChannel>("email");
  const [locale, setLocale] = useState("en");
  const [error, setError] = useState<string | null>(null);

  function reset() {
    setName("");
    setChannel("email");
    setLocale("en");
    setError(null);
  }

  async function submit() {
    setError(null);
    try {
      const template = await create.mutateAsync({
        name: name.trim() || "Untitled",
        channel,
        locale,
        subject: channel === "email" ? "Your verification code" : "",
        body: defaultBody(),
        note: "Draft created",
      });
      reset();
      onCreate(template);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to create template");
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) {
          reset();
          onClose();
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New template</DialogTitle>
          <DialogDescription>
            Starts as a draft at v1. Nothing renders from it until you publish.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3.5">
          <div className="grid gap-1.5">
            <Label htmlFor="tpl-name">Name</Label>
            <Input
              id="tpl-name"
              placeholder="Password reset"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="grid gap-1.5">
              <Label htmlFor="tpl-channel">Channel</Label>
              <SimpleSelect
                id="tpl-channel"
                value={channel}
                onValueChange={(v) => setChannel(v as TemplateChannel)}
                options={[
                  { value: "email", label: "Email" },
                  { value: "sms", label: "SMS" },
                ]}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="tpl-locale">Locale</Label>
              <SimpleSelect
                id="tpl-locale"
                value={locale}
                onValueChange={setLocale}
                options={[
                  { value: "en", label: "en - English" },
                  { value: "vi", label: "vi - Tiếng Việt" },
                ]}
              />
            </div>
          </div>
          {error && <p className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              reset();
              onClose();
            }}
          >
            Cancel
          </Button>
          <Button size="sm" onClick={submit} disabled={!name.trim() || create.isPending}>
            <FileText className="size-3.5" />
            Create draft
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
