/**
 * One place to turn raw API values into human language.
 * Components must never print SNAKE_CASE enum values directly.
 */

export type Tone = 'success' | 'warning' | 'danger' | 'info' | 'neutral';

export interface StatusMeta {
  label: string;
  tone: Tone;
}

const STATUS_MAP: Record<string, StatusMeta> = {
  // Exception case lifecycle
  OPEN: { label: 'Open', tone: 'info' },
  COLLECTING_EVIDENCE: { label: 'Collecting evidence', tone: 'info' },
  INVESTIGATING: { label: 'Investigating', tone: 'info' },
  AWAITING_MERCHANT_APPROVAL: { label: 'Needs approval', tone: 'warning' },
  NEEDS_APPROVAL: { label: 'Needs approval', tone: 'warning' },
  ESCALATED: { label: 'Escalated', tone: 'danger' },
  RESOLVED: { label: 'Resolved', tone: 'success' },
  REJECTED: { label: 'Rejected', tone: 'danger' },
  CLOSED: { label: 'Closed', tone: 'neutral' },

  // Payment / order states
  PAID: { label: 'Paid', tone: 'success' },
  COMPLETED: { label: 'Paid', tone: 'success' },
  CAPTURED: { label: 'Captured', tone: 'success' },
  PAYMENT_PENDING: { label: 'Payment pending', tone: 'warning' },
  PENDING: { label: 'Pending', tone: 'neutral' },
  PROCESSING: { label: 'Processing', tone: 'info' },
  FAILED: { label: 'Failed', tone: 'danger' },
  DECLINED: { label: 'Declined', tone: 'danger' },
  REFUNDED: { label: 'Refunded', tone: 'neutral' },
  CANCELLED: { label: 'Cancelled', tone: 'neutral' },
  UNKNOWN: { label: 'Unknown', tone: 'neutral' },

  // Assistant drafts
  DRAFTED: { label: 'Drafted', tone: 'warning' },
  APPROVED: { label: 'Approved', tone: 'info' },
  DISCARDED: { label: 'Discarded', tone: 'neutral' },
};

const CASE_TYPE_MAP: Record<string, string> = {
  PROVIDER_MERCHANT_STATE_MISMATCH: 'Payment and order out of sync',
  PROVIDER_ORDER_MISMATCH: 'Payment and order out of sync',
  AMOUNT_MISMATCH: "Amounts don't match",
  CURRENCY_MISMATCH: "Currencies don't match",
  DUPLICATE_CHARGE: 'Possible duplicate charge',
  PAYMENT_NOT_RECEIVED: 'Payment not received',
  ORDER_NOT_FOUND: 'Order not found',
  REFUND_NOT_RECEIVED: 'Refund not received',
  REFUND_ISSUE: 'Refund problem',
  OTHER: 'Something else',
};

const CATEGORY_MAP: Record<string, string> = {
  ORDER_ALREADY_RECONCILED: 'Order already reconciled',
  PROVIDER_MERCHANT_STATE_MISMATCH: 'Payment and order out of sync',
  PAYMENT_STATE_MISMATCH: 'Payment and order out of sync',
  AMOUNT_MISMATCH: "Amounts don't match",
  CURRENCY_MISMATCH: "Currencies don't match",
  DUPLICATE_CHARGE: 'Possible duplicate charge',
  PAYMENT_FAILED: 'Payment failed',
  PAYMENT_PENDING: 'Payment still pending',
  WEBHOOK_DELAY: 'Delayed payment update',
  MISSING_ORDER: 'Order missing from the store',
  REFUND_ISSUE: 'Refund problem',
  UNKNOWN: 'Needs a closer look',
};

const EVENT_MAP: Record<string, string> = {
  CASE_OPENED: 'Case opened',
  CASE_CREATED: 'Case opened',
  CASED_OPENED: 'Case opened',
  EVIDENCE_COLLECTED: 'Evidence collected',
  EVIDENCE_ADDED: 'Evidence added',
  INVESTIGATION_STARTED: 'Investigation started',
  INVESTIGATION_COMPLETED: 'AI investigation completed',
  INVESTIGATION_TRIGGERED: 'Investigation started',
  DECISION_MADE: 'Policy decision made',
  POLICY_DECISION: 'Policy decision made',
  ACTION_PROPOSED: 'Reconcile requested',
  ACTION_EXECUTED: 'Order reconciled',
  ORDER_RECONCILED: 'Order reconciled',
  STATUS_CHANGED: 'Status updated',
  MESSAGE_ADDED: 'Message added',
  MESSAGE_SENT: 'Message added',
  COMMENT_ADDED: 'Message added',
  PAYMENT_UPDATED: 'Payment updated',
  WEBHOOK_RECEIVED: 'Payment update received',
  APPROVAL_REQUESTED: 'Approval requested',
  APPROVAL_DECIDED: 'Approval decided',
  CASE_RESOLVED: 'Case resolved',
  CASE_CLOSED: 'Case closed',
  AI_MESSAGE_SENT: 'AI-assisted message sent',
  INCIDENT_GROUPED: 'Grouped into an incident',
  DRAFT_CREATED: 'Reply drafted',
  DRAFT_APPROVED: 'Reply approved',
};

const DECISION_MAP: Record<string, StatusMeta> = {
  ALLOW: { label: 'Approved', tone: 'success' },
  BLOCK: { label: 'Blocked', tone: 'danger' },
  REQUIRE_APPROVAL: { label: 'Needs approval', tone: 'warning' },
  WAIT: { label: 'Waiting', tone: 'info' },
};

const RISK_MAP: Record<string, StatusMeta> = {
  LOW: { label: 'Low risk', tone: 'success' },
  MEDIUM: { label: 'Medium risk', tone: 'warning' },
  HIGH: { label: 'High risk', tone: 'danger' },
  CRITICAL: { label: 'Critical risk', tone: 'danger' },
};

const DRAFT_TYPE_MAP: Record<string, string> = {
  STATUS_UPDATE: 'Status update',
  INFORMATION_REQUEST: 'Information request',
  RESOLUTION_NOTICE: 'Resolution notice',
};

/**
 * Turn an API/client error into a sentence a person can act on.
 * Backend chapters arrive as JSON problem documents; prefer `detail`, then `title`.
 */
export function apiErrorMessage(
  error: unknown,
  fallback = 'Something went wrong. Try again.',
): string {
  if (error instanceof Error) {
    const raw = error.message.replace(/^API error \d+:\s*/, '').trim();
    try {
      const parsed = JSON.parse(raw) as { detail?: unknown; title?: unknown; message?: unknown };
      const message = parsed.detail ?? parsed.title ?? parsed.message;
      if (typeof message === 'string' && message.trim()) return message;
    } catch {
      // Not JSON: use the cleaned text below.
    }
    const cleaned = raw.replace(/^"|"$/g, '').trim();
    if (cleaned && !cleaned.startsWith('{')) return cleaned;
  }
  return fallback;
}

/** Sentence-case a raw enum without printing SNAKE_CASE. */
export function sentenceCase(raw: string | null | undefined): string {
  if (!raw) return '—';
  const words = raw.replace(/[_-]+/g, ' ').trim().toLowerCase();
  if (!words) return '—';
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function statusMeta(status: string | null | undefined): StatusMeta {
  if (!status) return { label: 'Unknown', tone: 'neutral' };
  const key = status.toUpperCase();
  return STATUS_MAP[key] ?? { label: sentenceCase(status), tone: 'neutral' };
}

export function formatStatus(status: string | null | undefined): string {
  return statusMeta(status).label;
}

export function formatCaseType(type: string | null | undefined): string {
  if (!type) return 'Payment issue';
  return CASE_TYPE_MAP[type.toUpperCase()] ?? sentenceCase(type);
}

export function formatCategory(category: string | null | undefined): string {
  if (!category) return 'Needs a closer look';
  return CATEGORY_MAP[category.toUpperCase()] ?? sentenceCase(category);
}

export function formatDecision(outcome: string | null | undefined): StatusMeta {
  if (!outcome) return { label: 'No decision', tone: 'neutral' };
  return DECISION_MAP[outcome.toUpperCase()] ?? { label: sentenceCase(outcome), tone: 'neutral' };
}

export function formatRisk(severity: string | null | undefined): StatusMeta {
  if (!severity) return { label: 'Not scored', tone: 'neutral' };
  return RISK_MAP[severity.toUpperCase()] ?? { label: `${sentenceCase(severity)} risk`, tone: 'neutral' };
}

/** Human label for an assistant draft type. */
export function formatDraftType(draftType: string | null | undefined): string {
  if (!draftType) return 'Reply';
  return DRAFT_TYPE_MAP[draftType.toUpperCase()] ?? sentenceCase(draftType);
}

export function formatValidation(status: string | null | undefined): StatusMeta {  switch ((status ?? '').toUpperCase()) {
    case 'REFS_VERIFIED':
      return { label: 'Verified', tone: 'success' };
    case 'SCHEMA_VALID':
      return { label: 'Checked', tone: 'info' };
    case 'REJECTED':
      return { label: 'Rejected', tone: 'danger' };
    default:
      return { label: 'Unverified', tone: 'neutral' };
  }
}

export function formatEventType(eventType: string | null | undefined): string {
  if (!eventType) return 'Update';
  return EVENT_MAP[eventType.toUpperCase()] ?? sentenceCase(eventType);
}

export function formatActor(actorType: string | null | undefined): string {
  switch ((actorType ?? '').toUpperCase()) {
    case 'CUSTOMER':
      return 'Customer';
    case 'MERCHANT_USER':
    case 'MERCHANT':
      return 'Merchant';
    case 'SYSTEM':
    case 'SERVICE':
      return 'FRACTAL';
    default:
      return 'System';
  }
}

/** Customer-visible activity is marked "CUSTOMER" by the API. */
export function isCustomerVisible(visibility: string | null | undefined): boolean {
  return (visibility ?? '').toUpperCase() === 'CUSTOMER';
}

export function formatMinorUnits(amountMinor: number | null | undefined, currency = 'USD'): string {
  if (amountMinor === null || amountMinor === undefined || Number.isNaN(amountMinor)) return '—';
  const code = (currency || 'USD').toUpperCase();
  try {
    const value = new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: code,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(amountMinor / 100);
    return `${value} ${code}`;
  } catch {
    return `${(amountMinor / 100).toFixed(2)} ${code}`;
  }
}

export function shortId(id: string | null | undefined): string {
  if (!id) return '—';
  return id.length > 8 ? id.slice(0, 8) : id;
}

export function formatTime(input: string | Date | null | undefined): string {
  if (!input) return '—';
  const date = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(date.getTime())) return '—';

  const diff = date.getTime() - Date.now();
  const abs = Math.abs(diff);
  const minute = 60_000;
  const hour = 60 * minute;
  const day = 24 * hour;

  if (abs < 45_000) return 'Just now';
  if (abs < hour) {
    const mins = Math.round(abs / minute);
    return `${mins} min ${diff < 0 ? 'ago' : 'from now'}`;
  }
  if (abs < day) {
    const hours = Math.round(abs / hour);
    return `${hours} hr ${diff < 0 ? 'ago' : 'from now'}`;
  }
  if (abs < 7 * day) {
    const days = Math.round(abs / day);
    return `${days} day${days === 1 ? '' : 's'} ${diff < 0 ? 'ago' : 'from now'}`;
  }
  return formatDate(date);
}

export function formatDate(input: string | Date | null | undefined): string {
  if (!input) return '—';
  const date = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(date.getTime())) return '—';
  const sameYear = date.getFullYear() === new Date().getFullYear();
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    year: sameYear ? undefined : 'numeric',
  }).format(date);
}

export function formatAbsolute(input: string | Date | null | undefined): string {
  if (!input) return '—';
  const date = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}

export function formatClock(input: string | Date | null | undefined): string {
  if (!input) return '—';
  const date = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit' }).format(date);
}

export function initials(name: string | null | undefined): string {
  if (!name) return '?';
  const parts = name.trim().split(/\s+/).slice(0, 2);
  return parts.map((part) => part.charAt(0).toUpperCase()).join('') || '?';
}

/** Risk level for the anomaly detector shown to admins on the merchant view. */
export function anomalyMeta(score: number, threshold: number): StatusMeta {
  return score >= threshold
    ? { label: 'Elevated', tone: 'warning' }
    : { label: 'Normal', tone: 'success' };
}
