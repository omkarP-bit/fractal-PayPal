import { useAuthStore } from '../store/auth';

const API_BASE = import.meta.env.VITE_API_BASE || '';

/** Row returned by GET /api/v1/cases. */
export interface CaseSummary {
  id: string;
  case_number: string;
  status: string;
  case_type: string;
  opened_via: string;
  version: number;
  created_at: string;
  updated_at: string;
  merchant: Party;
  customer: Party;
  payment: PaymentInfo | null;
  order: OrderInfo | null;
  summary: string;
  next_step: string;
  latest_decision: Decision | null;
}

export interface Party {
  id: string;
  name?: string;
  email?: string;
}

export interface PaymentInfo {
  status: string;
  amount_minor: number;
  currency: string;
  provider?: string;
  provider_order_id?: string;
  capture_id?: string;
  fetched_at?: string;
}

export interface OrderInfo {
  ref: string;
  status: string;
  amount_minor: number;
  currency: string;
  fetched_at?: string;
}

export interface DecisionFinding {
  rule_id?: string;
  code?: string;
  message?: string;
  passed?: boolean;
  result?: string;
}

export interface Decision {
  id: string;
  action_type?: string;
  outcome: string;
  policy_revision: string;
  findings: DecisionFinding[];
  created_at?: string;
  missing_gates?: string[];
  required_approvals?: string[];
}

export interface Investigation {
  category: string;
  root_cause_hypothesis: string;
  evidence_ids: string[];
  suggested_severity: string;
  validation_status?: string;
  recommendation?: string;
  created_at?: string;
}

export interface EvidenceItem {
  id: string;
  evidence_type: string;
  source_system: string;
  source_ref: string;
  fetched_at: string;
}

export interface ActivityItem {
  id: number;
  actor_type: string;
  event_type: string;
  visibility: string;
  data?: Record<string, unknown>;
  created_at: string;
}

export interface MessageItem {
  id: string;
  author_type: string;
  visibility: string;
  body: string;
  created_at: string;
  /** True when the assistant drafted the message. It is labeled to the customer. */
  ai_generated?: boolean;
  approved_by?: string;
  sent_at?: string | null;
}

export interface Approval {
  id: string;
  action_type: string;
  decision: string;
  expires_at: string;
}

export interface ProviderInfo {
  name: string;
  order_id: string;
  capture_id: string;
  fetched_at: string;
}

export interface Anomaly {
  score: number;
  model_id: string;
  model_version?: string;
  window_start?: string;
  window_end?: string;
  calculated_at: string;
  threshold: number;
  features?: Record<string, number | null>;
}

/** Full payload returned by GET /api/v1/cases/{id}. */
export interface CaseDetail extends CaseSummary {
  provider: ProviderInfo | null;
  investigation: Investigation | null;
  evidence: EvidenceItem[];
  activity: ActivityItem[];
  messages: MessageItem[];
  pending_approvals: Approval[];
  available_actions: string[];
  anomaly?: Anomaly | null;
  /** Incident this case is grouped into. Merchant only. */
  incident?: IncidentLink | null;
  /** Merchant-reviewable replies for this case. Merchant only. */
  drafts?: MessageDraft[];
}

/** The incident a case is grouped into. */
export interface IncidentLink {
  id: string;
  number: string;
  status: string;
  severity: string;
  priority_score: number;
  case_count: number;
  open_case_count: number;
  root_cause: string;
  summary: string;
  next_step: string;
}

/** A case inside an incident, with its triage facts. */
export interface IncidentCase {
  case_id: string;
  case_number: string;
  status: string;
  case_type: string;
  customer_name: string;
  customer_email: string;
  merchant_order_ref: string;
  provider_status: string;
  order_status: string;
  amount_minor: number;
  currency: string;
  severity: string;
  priority_score: number;
  age_minutes: number;
  summary: string;
  next_step: string;
  created_at: string;
}

/** A grouped root cause affecting many customer cases. */
export interface Incident {
  id: string;
  number: string;
  merchant_id: string;
  provider_account_id: string;
  root_cause_code: string;
  root_cause_summary: string;
  ai_summary: string;
  next_step: string;
  status: string;
  severity: string;
  confidence: number;
  priority_score: number;
  case_count: number;
  open_case_count: number;
  affected_amount_minor: number;
  currency: string;
  first_seen_at: string;
  last_seen_at: string;
  resolved_at?: string | null;
  created_at: string;
  updated_at: string;
  draft_type: string;
  drafts_pending: number;
  drafts_approved: number;
  drafts_sent: number;
  cases: IncidentCase[];
}

/** A customer reply the assistant drafted from verified case facts. */
export interface MessageDraft {
  id: string;
  case_id: string;
  incident_id: string;
  draft_type: string;
  body: string;
  status: string;
  evidence_refs: string[];
  ai_generated: boolean;
  guardrail_flag: string;
  approved_by?: string | null;
  approved_at?: string | null;
  sent_at?: string | null;
  created_at: string;
  updated_at: string;
  case_number?: string;
  customer_name?: string;
  customer_email?: string;
}

export interface TriageInbox {
  incidents: Incident[];
  cases: IncidentCase[];
  generated_at: string;
}

export interface IncidentDetail {
  incident: Incident;
  drafts: MessageDraft[];
}

export interface DraftResponse {
  drafts: MessageDraft[];
}

export interface SendResult {
  draft_id: string;
  message_id: string;
  case_id: string;
  case_number: string;
  sent_at: string;
  auto_sent: boolean;
}

export interface FanOutResult {
  incident_id: string;
  results: {
    case_id: string;
    case_number: string;
    status: string;
    outcome: string;
    detail: string;
  }[];
}

/** Response from POST /api/v1/cases. */
export interface Case {
  id: string;
  case_number: string;
  status: string;
  merchant_id: string;
  customer_id: string;
  payment_transaction_id: string;
  merchant_order_ref: string;
  case_type: string;
  opened_via: string;
  resolution_code: string | null;
  version: number;
  created_at: string;
  updated_at: string;
  closed_at: string | null;
}

export interface CreateCaseRequest {
  merchant_id: string;
  merchant_order_ref: string;
  claim: { email: string; token: string };
  case_type: string;
  description: string;
}

export interface Region {
  code: string;
  name: string;
  currency: string;
  currencies: string[];
  cross_border_only: boolean;
  note?: string;
}

export interface MerchantSummary {
  id: string;
  legal_name: string;
  country: string;
  currency: string;
  onboarding_status: string;
}

export interface ProviderAccountSummary {
  id: string;
  provider: string;
  environment: string;
  client_id?: string;
  managed: boolean;
  webhook_id?: string;
  webhook_url?: string;
  webhook_status: string;
  status: string;
  webhook_registered_at?: string;
  last_checked_at?: string;
  webhook_error?: string;
}

export interface OnboardingStatus {
  merchant: MerchantSummary;
  provider_account: ProviderAccountSummary | null;
  ready: boolean;
  next_step: 'connect_paypal' | 'retry_webhook' | 'done';
}

export interface SignupRequest {
  email: string;
  password: string;
  display_name: string;
  legal_name: string;
  country: string;
  currency: string;
}

export interface SignupResponse {
  token: string;
  user: {
    sub: string;
    kind: string;
    merchant_id: string;
    merchant_name: string;
    role: string;
    permissions: string[];
    email: string;
    display_name: string;
  };
  merchant: MerchantSummary;
  next_step: string;
}

export interface WebhookEventSummary {
  event_id: string;
  event_type: string;
  provider_account_id: string;
  signature_verified: boolean;
  processing_status: string;
  received_at: string;
}

export interface ObservabilitySummary {
  generated_at: string;
  merchant_id: string;
  kibana_url: string;
  provider_account: ProviderAccountSummary | null;
  webhook_events: {
    total: number;
    last_24h: number;
    last_received: string | null;
  };
  recent_events: WebhookEventSummary[];
}

interface ActionProposal {
  action_type: string;
  params: { amount_minor: number; currency: string };
}

export interface ActionResponse {
  decision: Decision;
  execution: { id: string; status: string; completed_at: string | null } | null;
  approval: { id: string; decision: string; params_sha256: string } | null;
  case: { status: string; version: number };
}

class ApiClient {
  private baseURL: string;

  constructor(baseURL: string) {
    this.baseURL = baseURL;
  }

  private getToken(): string | null {
    return useAuthStore.getState().token;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const token = this.getToken();
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string>),
    };
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(`${this.baseURL}${path}`, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`API error ${response.status}: ${errorText}`);
    }

    if (response.status === 204) {
      return undefined as T;
    }

    return response.json();
  }

  // Auth
  async devLogin(role: 'CUSTOMER' | 'MERCHANT', email?: string): Promise<{ token: string; user: any }> {
    return this.request<{ token: string; user: any }>('/api/v1/auth/dev-login', {
      method: 'POST',
      body: JSON.stringify({ role, email }),
    });
  }

  async loginWithPassword(email: string, password: string): Promise<{ token: string; user: any }> {
    return this.request<{ token: string; user: any }>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
  }

  // Merchant onboarding
  async getRegions(): Promise<Region[]> {
    const response = await this.request<{ regions: Region[] }>('/api/v1/onboarding/regions');
    return response.regions;
  }

  async signup(request: SignupRequest): Promise<SignupResponse> {
    return this.request<SignupResponse>('/api/v1/onboarding/signup', {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  async getOnboardingStatus(): Promise<OnboardingStatus> {
    return this.request<OnboardingStatus>('/api/v1/onboarding/status');
  }

  async connectPayPal(request: {
    client_id: string;
    client_secret: string;
    environment: string;
  }): Promise<{ provider_account: ProviderAccountSummary; merchant?: MerchantSummary }> {
    return this.request<{ provider_account: ProviderAccountSummary; merchant?: MerchantSummary }>(
      '/api/v1/onboarding/paypal',
      {
        method: 'POST',
        body: JSON.stringify(request),
      },
    );
  }

  async connectManagedPayPal(): Promise<{ provider_account: ProviderAccountSummary; merchant?: MerchantSummary }> {
    return this.request<{ provider_account: ProviderAccountSummary; merchant?: MerchantSummary }>(
      '/api/v1/onboarding/paypal/managed',
      { method: 'POST' },
    );
  }

  async reRegisterWebhook(): Promise<{ provider_account: ProviderAccountSummary }> {
    return this.request<{ provider_account: ProviderAccountSummary }>(
      '/api/v1/onboarding/paypal/re-register',
      { method: 'POST' },
    );
  }

  async getObservability(): Promise<ObservabilitySummary> {
    return this.request<ObservabilitySummary>('/api/v1/observability/summary');
  }

  // Cases
  async getCases(): Promise<CaseSummary[]> {
    return this.request<CaseSummary[]>('/api/v1/cases');
  }

  async getCase(id: string): Promise<CaseDetail> {
    return this.request<CaseDetail>(`/api/v1/cases/${id}`);
  }

  async createCase(request: CreateCaseRequest): Promise<Case> {
    return this.request<Case>('/api/v1/cases', {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  async proposeAction(caseId: string, action: ActionProposal): Promise<ActionResponse> {
    return this.request<ActionResponse>(`/api/v1/cases/${caseId}/actions`, {
      method: 'POST',
      body: JSON.stringify(action),
    });
  }

  async getCaseTimeline(caseId: string): Promise<any[]> {
    return this.request<any[]>(`/api/v1/cases/${caseId}/timeline`);
  }

  async getCaseMessages(caseId: string): Promise<any[]> {
    return this.request<any[]>(`/api/v1/cases/${caseId}/messages`);
  }

  async createCaseMessage(caseId: string, body: string, visibility: string = 'SHARED'): Promise<any> {
    return this.request<any>(`/api/v1/cases/${caseId}/messages`, {
      method: 'POST',
      body: JSON.stringify({ body, visibility }),
    });
  }

  // AI Case Assistant
  async getTriageInbox(): Promise<TriageInbox> {
    return this.request<TriageInbox>('/api/v1/assistant/incidents');
  }

  async getIncident(id: string): Promise<IncidentDetail> {
    return this.request<IncidentDetail>(`/api/v1/assistant/incidents/${id}`);
  }

  async regroupCases(): Promise<{ incidents: Incident[] }> {
    return this.request<{ incidents: Incident[] }>('/api/v1/assistant/regroup', { method: 'POST' });
  }

  async groupCase(caseId: string): Promise<{ incident: Incident | null }> {
    return this.request<{ incident: Incident | null }>(`/api/v1/cases/${caseId}/group`, {
      method: 'POST',
    });
  }

  async draftIncidentReplies(id: string): Promise<DraftResponse> {
    return this.request<DraftResponse>(`/api/v1/assistant/incidents/${id}/drafts`, { method: 'POST' });
  }

  async draftCaseReply(caseId: string): Promise<DraftResponse> {
    return this.request<DraftResponse>(`/api/v1/cases/${caseId}/drafts`, { method: 'POST' });
  }

  async approveIncidentDrafts(id: string): Promise<DraftResponse> {
    return this.request<DraftResponse>(`/api/v1/assistant/incidents/${id}/drafts/approve`, {
      method: 'POST',
    });
  }

  async sendIncidentDrafts(id: string): Promise<{ sent: SendResult[] }> {
    return this.request<{ sent: SendResult[] }>(`/api/v1/assistant/incidents/${id}/drafts/send`, {
      method: 'POST',
    });
  }

  async approveDraft(id: string): Promise<DraftResponse> {
    return this.request<DraftResponse>(`/api/v1/assistant/drafts/${id}/approve`, { method: 'POST' });
  }

  async updateDraft(id: string, body: string): Promise<DraftResponse> {
    return this.request<DraftResponse>(`/api/v1/assistant/drafts/${id}`, {
      method: 'POST',
      body: JSON.stringify({ body }),
    });
  }

  async sendDraft(id: string): Promise<{ sent: SendResult }> {
    return this.request<{ sent: SendResult }>(`/api/v1/assistant/drafts/${id}/send`, { method: 'POST' });
  }

  async discardDraft(id: string): Promise<{ status: string }> {
    return this.request<{ status: string }>(`/api/v1/assistant/drafts/${id}/discard`, {
      method: 'POST',
    });
  }

  async removeIncidentCase(incidentId: string, caseId: string): Promise<{ incident: Incident }> {
    return this.request<{ incident: Incident }>(
      `/api/v1/assistant/incidents/${incidentId}/cases/${caseId}`,
      { method: 'DELETE' },
    );
  }

  async fanOutIncident(incidentId: string): Promise<FanOutResult> {
    return this.request<FanOutResult>(`/api/v1/assistant/incidents/${incidentId}/fanout`, {
      method: 'POST',
    });
  }

  // Telemetry (agent tools)
  async getLogsByCase(caseId: string, window: string = '1h'): Promise<any> {
    return this.request<any>(`/internal/v1/telemetry/logs?case_id=${caseId}&window=${window}`);
  }

  async getTrace(traceId: string): Promise<any> {
    return this.request<any>(`/internal/v1/telemetry/traces/${traceId}`);
  }

  async getHandlerErrorRate(merchantId: string, window: string = '24h'): Promise<any> {
    return this.request<any>(
      `/internal/v1/telemetry/handler-error-rate?merchant_id=${merchantId}&window=${window}`,
    );
  }
}

export const apiClient = new ApiClient(API_BASE);
