"use client";

import { useState } from "react";
import { FileText } from "lucide-react";
import type { Template } from "@/lib/roadmap/templates";
import { TEMPLATES } from "@/lib/roadmap/templates";
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

export function NewTemplateDialog({
  open,
  onClose,
  onCreate,
}: {
  open: boolean;
  onClose: () => void;
  onCreate: (t: Template) => void;
}) {
  const [name, setName] = useState("");
  const [channel, setChannel] = useState<"email" | "sms">("email");
  const [locale, setLocale] = useState("en");
  const [from, setFrom] = useState("blank");

  const starters = [{ value: "blank", label: "Blank" }].concat(
    TEMPLATES.map((t) => ({
      value: t.id,
      label: `${t.name} · ${t.channel} · ${t.locale}`,
    })),
  );

  function reset() {
    setName("");
    setChannel("email");
    setLocale("en");
    setFrom("blank");
  }

  function create() {
    const base = TEMPLATES.find((t) => t.id === from);
    const slug =
      name
        .trim()
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "_")
        .replace(/^_|_$/g, "") || "untitled";
    onCreate({
      id: `tpl_${slug}`,
      name: name.trim() || "Untitled",
      channel,
      locale,
      version: 1,
      status: "expired",
      updated: "just now",
      subject: base
        ? base.subject
        : channel === "email"
          ? "Your worklane code: {{code}}"
          : "-",
      body: base
        ? base.body
        : channel === "email"
          ? "Your worklane verification code is {{code}}. It expires in 5 minutes."
          : "worklane: {{code}} is your code. Expires in 5 min.",
      versions: [
        {
          v: 1,
          when: "just now",
          by: "you",
          note: base ? `Forked from ${base.id}` : "Draft created",
        },
      ],
      sends: [],
    });
    reset();
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
                onValueChange={(v) => setChannel(v as "email" | "sms")}
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
          <div className="grid gap-1.5">
            <Label htmlFor="tpl-from">Start from</Label>
            <SimpleSelect
              id="tpl-from"
              value={from}
              onValueChange={setFrom}
              options={starters}
            />
            <span className="text-xs text-muted-foreground">
              Forking copies the body and subject; version history starts fresh.
            </span>
          </div>
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
          <Button size="sm" onClick={create} disabled={!name.trim()}>
            <FileText className="size-3.5" />
            Create draft
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
