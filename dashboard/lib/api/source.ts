import type {
  ApiKey,
  DeliveryLog,
  Overview,
  OtpRequest,
  SendResult,
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
  send(recipient: string): Promise<SendResult>;
  verify(recipient: string, code: string): Promise<VerifyResult>;
}
