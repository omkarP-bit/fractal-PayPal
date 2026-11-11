import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ArrowLeft, Check, X } from 'lucide-react';
import { apiClient, type CaseDetail } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { useSSE } from '../../hooks/useSSE';
import { cn } from '../../lib/cn';
import {
  formatAbsolute,
  formatActor,
  formatEventType,
  formatMinorUnits,
  formatTime,
  isCustomerVisible,
  statusMeta,
} from '../../lib/format';
import { Button } from '../../components/ui/Button';
import { StatusPill } from '../../components/ui/StatusPill';
import { Tooltip } from '../../components/ui/Tooltip';
import { Stepper, type Step } from '../../components/ui/Stepper';
import { Timeline, type TimelineEntry } from '../../components/ui/Timeline';
import { ErrorNotice, Skeleton } from '../../components/ui/States';
import { Conversation } from '../../components/Conversation';

const PANEL = 'panel';

const HEADLINES: Record<string, string> = {
  OPEN: 'We’ve opened your case.',
  COLLECTING_EVIDENCE: 'We’re gathering the details.',
  INVESTIGATING: 'We’re checking your payment with PayPal.',
  AWAITING_MERCHANT_APPROVAL: 'Your merchant is reviewing the fix.',
  ESCALATED: 'A specialist is reviewing your case.',
  RESOLVED: 'Your payment is confirmed. Nothing else is needed.',
  REJECTED: 'We couldn’t resolve this case.',
  CLOSED: 'This case is closed.',
};

const SUPPORTING: Record<string, string> = {
  OPEN: 'We’ll keep you posted here as soon as there’s an update.',
  COLLECTING_EVIDENCE: 'We’re collecting the payment and order details we need to compare.',
  INVESTIGATING: 'This usually takes a few minutes. You don’t need to do anything right now.',
  AWAITING_MERCHANT_APPROVAL:
    'Your payment is safe. We’re waiting for the merchant to approve the change.',
  ESCALATED: 'A specialist is taking a closer look. We’ll update you here.',
  RESOLVED: 'We matched your payment to the merchant’s order. You can keep this case for your records.',
  REJECTED: 'There was not enough evidence to change the order. Contact your merchant if you need more help.',
  CLOSED: 'No further action is needed. You can still read the history of this case.',
};

function WhatYouCanDo() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (ref.current && !ref.current.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  return (
    <section ref={ref} className={cn(PANEL, 'relative p-5')}>
      <h2 className="text-body font-bold text-ink">What you can do</h2>
      <p className="mt-1 text-meta text-muted">Read, message and add evidence.</p>
      <Button
        variant="tertiary"
        size="sm"
        className="mt-1 h-8 px-0"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
      >
        Learn more
      </Button>
      {open && (
        <div className="absolute right-4 top-full z-30 mt-2 w-72 animate-slide-up rounded border-2 border-line bg-surface p-4 shadow-hard-lg">
          <p className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">
            You can
          </p>
          <ul className="mt-2 space-y-1.5">
            {['Read every update', 'Message your merchant', 'Add evidence to this case'].map((item) => (
              <li key={item} className="flex items-start gap-2 text-meta text-ink">
                <Check className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" strokeWidth={3} />
                {item}
              </li>
            ))}
          </ul>
          <p className="mt-4 text-[11px] font-bold uppercase tracking-[0.04em] text-muted">You cannot</p>
          <ul className="mt-2 space-y-1.5">
            {['Approve a refund', 'Change an order total', 'Edit PolicyGuard rules'].map((item) => (
              <li key={item} className="flex items-start gap-2 text-meta text-ink">
                <X className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" strokeWidth={3} />
                {item}
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}

export function CustomerCaseDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { logout } = useAuthStore();
  const [caseData, setCaseData] = useState<CaseDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchCase = useCallback(
    async (quiet = false) => {
      if (!id) return;
      if (!quiet) setLoading(true);
      try {
        const data = await apiClient.getCase(id);
        setCaseData(data);
        setError(null);
      } catch (err) {
        const message = err instanceof Error ? err.message : 'Failed to load case';
        setError(message);
        if (message.includes('401')) {
          logout();
          navigate('/login');
        }
      } finally {
        if (!quiet) setLoading(false);
      }
    },
    [id, logout, navigate],
  );

  useEffect(() => {
    void fetchCase();
  }, [fetchCase]);

  const onCaseEvent = useCallback(() => {
    void fetchCase(true);
  }, [fetchCase]);

  useSSE(id, onCaseEvent);

  const sendMessage = async (body: string) => {
    if (!id) return;
    await apiClient.createCaseMessage(id, body, 'SHARED');
    await fetchCase(true);
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-52 w-full rounded" />
        <div className="page-grid">
          <div className="page-grid-main space-y-5">
            <Skeleton className="h-40 w-full rounded" />
            <Skeleton className="h-72 w-full rounded" />
          </div>
          <div className="page-grid-side space-y-5">
            <Skeleton className="h-64 w-full rounded" />
            <Skeleton className="h-28 w-full rounded" />
          </div>
        </div>
      </div>
    );
  }

  if (!caseData) {
    return (
      <div className="space-y-5">
        <Link
          to="/customer/cases"
          className="inline-flex items-center gap-1.5 text-meta font-medium text-blue hover:underline"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          Back to cases
        </Link>
        <ErrorNotice
          title="We couldn’t load this case"
          description={error || 'The case may have been removed.'}
          onRetry={() => void fetchCase()}
        />
      </div>
    );
  }

  const status = caseData.status.toUpperCase();
  const payment = caseData.payment;
  const order = caseData.order;
  const resolved = ['RESOLVED', 'CLOSED', 'REJECTED'].includes(status);
  const currentIndex = resolved
    ? 3
    : status === 'COLLECTING_EVIDENCE'
      ? 1
      : status === 'OPEN'
        ? 0
        : 2;

  const labels = ['Opened', 'Evidence collected', 'Investigating', status === 'REJECTED' ? 'Closed' : 'Resolved'];
  const steps: Step[] = labels.map((label, index) => ({
    label,
    state: resolved ? 'complete' : index < currentIndex ? 'complete' : index === currentIndex ? 'current' : 'upcoming',
  }));

  const headline = HEADLINES[status] ?? 'We’re working on your case.';
  const supporting = caseData.summary?.trim() || SUPPORTING[status] || 'We’ll keep you updated here.';

  const timelineEntries: TimelineEntry[] =
    (caseData.activity ?? [])
      .filter((item) => isCustomerVisible(item.visibility))
      .map((item) => ({
        id: item.id,
        time: item.created_at,
        title: formatEventType(item.event_type),
        actor: formatActor(item.actor_type),
        customerVisible: true,
        detail:
          item.data && typeof item.data.message === 'string' ? item.data.message : undefined,
      }));

  return (
    <div className="space-y-5">
      <div>
        <Link
          to="/customer/cases"
          className="inline-flex items-center gap-1.5 text-meta font-medium text-blue hover:underline"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          Back to cases
        </Link>
        <div className="mt-2 flex flex-wrap items-center gap-2.5">
          <h1 className="text-title font-extrabold text-ink">Case {caseData.case_number}</h1>
          <span key={caseData.status} className="animate-fade-in" aria-live="polite">
            <StatusPill meta={statusMeta(caseData.status)} />
          </span>
        </div>
        <p className="mt-0.5 text-meta text-muted">
          With {caseData.merchant?.name || 'your merchant'}. Opened{' '}
          <Tooltip content={formatAbsolute(caseData.created_at)}>
            <time dateTime={caseData.created_at}>{formatTime(caseData.created_at)}</time>
          </Tooltip>
        </p>
      </div>

      <div className="page-grid">
        <div className="page-grid-main space-y-5">
          <section className={cn(PANEL, 'p-6')}>
            <h2 className="max-w-prose text-[24px] font-extrabold leading-8 text-ink">{headline}</h2>
            <p className="mt-2 max-w-prose text-body text-muted">{supporting}</p>
            {caseData.next_step && caseData.next_step !== caseData.summary && (
              <p className="mt-1 max-w-prose text-body text-muted">{caseData.next_step}</p>
            )}
            <div className="mt-6 border-t-2 border-line pt-6">
              <Stepper steps={steps} />
            </div>
          </section>

          <section className={cn(PANEL, 'p-6')}>
            <h2 className="text-section font-bold text-ink">Payment and order</h2>
            <div className="cq mt-4">
              <div className="split-panel rounded border-2 border-line">
                <div className="p-4 sm:p-5">
                  <p className="section-tag text-muted">PayPal</p>
                  <p className="mt-1 text-hero font-extrabold tabular-nums text-ink">
                    {payment ? formatMinorUnits(payment.amount_minor, payment.currency) : '—'}
                  </p>
                  <div className="mt-2">
                    <StatusPill meta={statusMeta(payment?.status)} />
                  </div>
                </div>
                <div className="p-4 sm:p-5">
                  <p className="section-tag text-muted">Store order</p>
                  <p className="mt-1 break-all font-mono text-[18px] font-bold text-ink sm:text-[20px]">
                    {order?.ref ?? '—'}
                  </p>
                  <div className="mt-2">
                    <StatusPill meta={statusMeta(order?.status)} />
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className={cn(PANEL, 'p-6')}>
            <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
              <h2 className="text-section font-bold text-ink">Conversation</h2>
              <p className="text-[12px] text-muted">Messages are shared with your merchant.</p>
            </div>
            <Conversation
              messages={caseData.messages ?? []}
              perspective="customer"
              onSend={sendMessage}
              emptyText="No messages yet. Ask a question or share what happened."
            />
          </section>
        </div>

        <aside className="page-grid-side space-y-5">
          <section className={cn(PANEL, 'p-5')}>
            <h2 className="text-body font-bold text-ink">Case timeline</h2>
            <div className="mt-4">
              <Timeline
                entries={timelineEntries}
                showMerchantOnly={false}
                emptyText="Updates will appear here as your case moves forward."
              />
            </div>
          </section>

          <WhatYouCanDo />
        </aside>
      </div>
    </div>
  );
}
