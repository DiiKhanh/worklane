"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import { qk } from "./keys";

export function useSend() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (recipient: string) => getDataSource().send(recipient),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.requests });
      qc.invalidateQueries({ queryKey: qk.logs });
    },
  });
}

export function useVerify() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { recipient: string; code: string }) =>
      getDataSource().verify(vars.recipient, vars.code),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.requests });
    },
  });
}
