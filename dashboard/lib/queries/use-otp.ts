"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useSend() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { recipient: string; channel: string; locale?: string }) =>
      getDataSource().send(vars.recipient, vars.channel, vars.locale),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.requests });
      qc.invalidateQueries({ queryKey: qk.logs });
    },
  });
}

export function useVerify() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { recipient: string; code: string; channel: string }) =>
      getDataSource().verify(vars.recipient, vars.code, vars.channel),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.requests });
    },
  });
}
