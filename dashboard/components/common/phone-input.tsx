"use client";

import { useState } from "react";
import { DIAL_CODES, toE164, parsePhone } from "@/lib/phone";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

export function PhoneInput({
  value,
  onChange,
  id,
}: {
  value: string;
  onChange: (e164: string) => void;
  id?: string;
}) {
  const parsed = parsePhone(value);
  const [dialCode, setDialCode] = useState(parsed?.dialCode ?? DIAL_CODES[0].code);
  const [national, setNational] = useState(parsed?.national ?? "");

  const update = (code: string, num: string) => {
    setDialCode(code);
    setNational(num);
    onChange(toE164(code, num));
  };

  return (
    <div className="flex gap-2">
      <Select value={dialCode} onValueChange={(c) => update(c as string, national)}>
        <SelectTrigger className="w-28 shrink-0">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {DIAL_CODES.map((d) => (
            <SelectItem key={d.iso} value={d.code}>
              {d.code} {d.iso}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Input
        id={id}
        inputMode="tel"
        autoComplete="off"
        placeholder="901 234 567"
        value={national}
        onChange={(e) => update(dialCode, e.target.value)}
      />
    </div>
  );
}
