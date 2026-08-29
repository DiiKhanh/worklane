"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getDataSource } from "@/lib/api";
import type { AddVersionInput, CreateTemplateInput, PreviewInput } from "@/lib/api/types";
import { qk } from "./keys";

export function useTemplates() {
  return useQuery({ queryKey: qk.templates, queryFn: () => getDataSource().listTemplates() });
}

export function useTemplate(id: string) {
  return useQuery({
    queryKey: qk.template(id),
    queryFn: () => getDataSource().getTemplate(id),
    enabled: !!id,
  });
}

export function useCreateTemplate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTemplateInput) => getDataSource().createTemplate(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.templates }),
  });
}

export function useAddVersion(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddVersionInput) => getDataSource().addVersion(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.template(id) }),
  });
}

export function usePublishVersion(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (versionId: string) => getDataSource().publishVersion(id, versionId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: qk.template(id) });
      qc.invalidateQueries({ queryKey: qk.templates });
    },
  });
}

export function usePreview() {
  return useMutation({ mutationFn: (input: PreviewInput) => getDataSource().previewTemplate(input) });
}
