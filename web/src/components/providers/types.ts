export interface ModelTestResult {
  ok: boolean;
  latency_ms: number;
  timestamp: string;
  error?: string;
  message?: string;
}
