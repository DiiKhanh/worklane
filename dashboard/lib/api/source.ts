import type {
  AddVersionInput,
  ApiKey,
  CreateTemplateInput,
  DeliveryLog,
  Overview,
  OtpRequest,
  PreviewInput,
  PreviewResult,
  SendResult,
  Template,
  TemplateDetail,
  TemplateVersion,
  VerifyResult,
} from "./types";

/**
 * The single interface the UI talks to. Mock and live implementations are
 * interchangeable; screens never know which one they hold.
 */
export interface DataSource {
  listApiKeys(): Promise<ApiKey[]>;
  listRequests(): Promise<OtpRequest[]>;
  listLogs(): Promise<DeliveryLog[]>;
  getOverview(): Promise<Overview>;
  send(recipient: string, channel: string): Promise<SendResult>;
  verify(recipient: string, code: string, channel?: string): Promise<VerifyResult>;

  // Template Studio
  listTemplates(): Promise<Template[]>;
  getTemplate(id: string): Promise<TemplateDetail>;
  createTemplate(input: CreateTemplateInput): Promise<Template>;
  addVersion(id: string, input: AddVersionInput): Promise<TemplateVersion>;
  publishVersion(id: string, versionId: string): Promise<void>;
  previewTemplate(input: PreviewInput): Promise<PreviewResult>;
}
