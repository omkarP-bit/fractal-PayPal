import { useAuthStore } from '../store/auth';

const API_BASE = import.meta.env.VITE_API_BASE || 'http://localhost:8080';

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

interface CustomerCaseView {
  id: string;
  case_number: string;
  status: string;
  merchant: { id: string; name: string };
  payment: { status: string; amount_minor: number; currency: string };
  order: { ref: string; status: string };
  summary: string;
  next_step: string;
  version: number;
  updated_at: string;
}

interface MerchantCaseView extends CustomerCaseView {
  provider: { name: string; order_id: string; capture_id: string; fetched_at: string };
  investigation: {
    category: string;
    root_cause_hypothesis: string;
    evidence_ids: string[];
    suggested_severity: string;
    validation_status: string;
  };
  latest_decision: { id: string; outcome: string; policy_revision: string; findings: any[] };
  available_actions: string[];
  pending_approvals: any[];
}

interface CreateCaseRequest {
  merchant_id: string;
  merchant_order_ref: string;
  claim: { email: string; token: string };
  case_type: string;
  description: string;
}

interface ActionProposal {
  action_type: string;
  params: { amount_minor: number; currency: string };
}

interface ActionResponse {
  decision: { id: string; outcome: string; policy_revision: string; findings: any[]; missing_gates: string[]; required_approvals: string[] };
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
    return localStorage.getItem('fractal_token');
  }

  private async request<T>(
    path: string,
    options: RequestInit = {}
  ): Promise<T> {
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

  // Cases
  async getCases(): Promise<Case[]> {
    return this.request<Case[]>('/api/v1/cases');
  }

  async getCase(id: string): Promise<CustomerCaseView | MerchantCaseView> {
    return this.request<CustomerCaseView | MerchantCaseView>(`/api/v1/cases/${id}`);
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

  // Telemetry (agent tools)
  async getLogsByCase(caseId: string, window: string = '1h'): Promise<any> {
    return this.request<any>(`/internal/v1/telemetry/logs?case_id=${caseId}&window=${window}`);
  }

  async getTrace(traceId: string): Promise<any> {
    return this.request<any>(`/internal/v1/telemetry/traces/${traceId}`);
  }

  async getHandlerErrorRate(merchantId: string, window: string = '24h'): Promise<any> {
    return this.request<any>(`/internal/v1/telemetry/handler-error-rate?merchant_id=${merchantId}&window=${window}`);
  }
}

export const apiClient = new ApiClient(API_BASE);
