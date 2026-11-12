import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ArrowLeft,
  Check,
  Hash,
  History,
  Layers,
  Link2,
  MoreHorizontal,
  RefreshCw,
  Send,
  ShieldCheck,
  Sparkles,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { apiClient, type CaseDetail } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { useSSE } from '../../hooks/useSSE';
import { cn } from '../../lib/cn';
import {
  anomalyMeta,
  apiErrorMessage,
  formatAbsolute,
  formatActor,
  formatCategory,
  formatDecision,
  formatEventType,
  formatMinorUnits,
  formatRisk,
  formatTime,
  formatValidation,
  isCustomerVisible,
  sentenceCase,
  statusMeta,
} from '../../lib/format';
import { Button, IconButton } from '../../components/ui/Button';
import { StatusPill, Tag, TONE_SURFACE } from '../../components/ui/StatusPill';
import { CopyChip } from '../../components/ui/CopyChip';
import { Tooltip } from '../../components/ui/Tooltip';
import { Disclosure } from '../../components/ui/Disclosure';
import { ConfirmDialog } from '../../components/ui/Dialog';
import { ErrorNotice, Skeleton } from '../../components/ui/States';
import { Timeline, type TimelineEntry } from '../../components/ui/Timeline';
import { Conversation } from '../../components/Conversation';

const PANEL = 'panel';

function PermissionsCard() {
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
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-body font-bold text-ink">Your access</h2>
          <p className="mt-0.5 text-meta text-muted">Admin access</p>
        </div>
        <Button
          variant="tertiary"
          size="sm"
          onClick={() => setOpen((value) => !value)}
          aria-expanded={open}
        >
          View permissions
        </Button>
      </div>
      {open && (
        <div className="absolute right-4 top-full z-30 mt-2 w-72 animate-slide-up rounded border-2 border-line bg-surface p-4 shadow-hard-lg">
          <p className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">Can</p>
          <ul className="mt-2 space-y-1.5">
            {[
              'Reconcile orders',
              'Approve requests from other admins',
              'See the risk score for this case',
            ].map((item) => (
              <li key={item} className="flex items-start gap-2 text-meta text-ink">
                <Check className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" strokeWidth={3} />
                {item}
              </li>
            ))}
          </ul>
          <p className="mt-4 text-[11px] font-bold uppercase tracking-[0.04em] text-muted">Cannot</p>
          <ul className="mt-2 space-y-1.5">
            {['Edit PolicyGuard rules', 'See cases from other merchants'].map((item) => (
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

export function MerchantCaseDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { logout } = useAuthStore();
  const [caseData, setCaseData] = useState<CaseDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [reconciling, setReconciling] = useState(false);
  const [reconcileError, setReconcileError] = useState<string | null>(null);
  const [showMerchantOnly, setShowMerchantOnly] = useState(true);
  const [menuOpen, setMenuOpen] = useState(false);
  const [drafting, setDrafting] = useState(false);
  const [draftBusy, setDraftBusy] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  const fetchCase = useCallback(
    async (quiet = false) => {
      if (!id) return;
      if (!quiet) setLoading(true);
      try {
        const data = await apiClient.getCase(id);
        setCaseData(data);
        setError(null);
      } catch (err) {
        const raw = err instanceof Error ? err.message : '';
        setError(apiErrorMessage(err, 'We couldn’t load this case. Try again.'));
        if (raw.includes('401')) {
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

  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) setMenuOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [menuOpen]);

  const copyText = async (value: string, confirmation: string) => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(confirmation);
    } catch {
      toast.error('We could not copy that. Try selecting it instead.');
    }
    setMenuOpen(false);
  };

  const performReconcile = async () => {
    if (!id || !caseData) return;
    setReconciling(true);
    setReconcileError(null);
    try {
      const response = await apiClient.proposeAction(id, {
        action_type: 'RECONCILE_MERCHANT_ORDER',
        params: {
          amount_minor: caseData.payment?.amount_minor ?? 0,
          currency: caseData.payment?.currency ?? 'USD',
        },
      });
      setConfirmOpen(false);
      await fetchCase(true);

      const decision = response.decision;
      if (decision.outcome === 'ALLOW') {
        toast.success('Order reconciled', {
          description: 'PolicyGuard approved the change and your order is up to date.',
        });
      } else if (decision.outcome === 'REQUIRE_APPROVAL') {
        toast.warning('Approval needed', {
          description: 'PolicyGuard needs another admin to approve this change.',
        });
      } else if (decision.outcome === 'WAIT') {
        toast.info('Waiting on more evidence', {
          description: 'PolicyGuard will review this case again when new evidence arrives.',
        });
      } else {
        toast.error('Reconcile blocked', {
          description: 'PolicyGuard did not allow this change. The case is unchanged.',
        });
      }
    } catch (err) {
      setConfirmOpen(false);
      setReconcileError(apiErrorMessage(err, 'The reconcile request failed. Try again.'));
    } finally {
      setReconciling(false);
    }
  };

  const sendMessage = async (body: string) => {
    if (!id) return;
    await apiClient.createCaseMessage(id, body, 'SHARED');
    await fetchCase(true);
  };

  const draftReply = async () => {
    if (!id) return;
    setDrafting(true);
    try {
      await apiClient.draftCaseReply(id);
      await fetchCase(true);
      toast.success('Reply drafted', {
        description: 'Review the wording before it reaches the customer.',
      });
    } catch (err) {
      toast.error(apiErrorMessage(err, 'We couldn’t draft a reply.'));
    } finally {
      setDrafting(false);
    }
  };

  const approveDraft = async (draftId: string) => {
    setDraftBusy(`approve-${draftId}`);
    try {
      await apiClient.approveDraft(draftId);
      await fetchCase(true);
      toast.success('Reply approved');
    } catch (err) {
      toast.error(apiErrorMessage(err, 'We couldn’t approve that reply.'));
    } finally {
      setDraftBusy(null);
    }
  };

  const sendDraft = async (draftId: string) => {
    setDraftBusy(`send-${draftId}`);
    try {
      await apiClient.sendDraft(draftId);
      await fetchCase(true);
      toast.success('Reply sent', {
        description: 'The customer was notified and the send is on the audit timeline.',
      });
    } catch (err) {
      toast.error(apiErrorMessage(err, 'We couldn’t send that reply.'));
    } finally {
      setDraftBusy(null);
    }
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <Skeleton className="h-4 w-24" />
        <div className="flex items-end justify-between gap-4">
          <div className="space-y-2">
            <Skeleton className="h-8 w-48" />
            <Skeleton className="h-4 w-64" />
          </div>
          <Skeleton className="h-10 w-40 rounded-pill" />
        </div>
        <div className="page-grid">
          <div className="page-grid-main space-y-5">
            <Skeleton className="h-64 w-full rounded" />
            <Skeleton className="h-56 w-full rounded" />
            <Skeleton className="h-40 w-full rounded" />
          </div>
          <div className="page-grid-side space-y-5">
            <Skeleton className="h-80 w-full rounded" />
            <Skeleton className="h-24 w-full rounded" />
          </div>
        </div>
      </div>
    );
  }

  if (!caseData) {
    return (
      <div className="space-y-5">
        <Link
          to="/merchant/cases"
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

  const payment = caseData.payment;
  const order = caseData.order;
  const provider = caseData.provider;
  const decision = caseData.latest_decision;
  const investigation = caseData.investigation;
  const evidence = caseData.evidence ?? [];
  const customerName = caseData.customer?.name || caseData.customer?.email || 'Customer';

  const paymentPaid = ['COMPLETED', 'CAPTURED', 'PAID'].includes(
    (payment?.status ?? '').toUpperCase(),
  );
  const orderPaid = ['PAID', 'COMPLETED', 'FULFILLED', 'RECONCILED'].includes(
    (order?.status ?? '').toUpperCase(),
  );
  const statusesDiffer = Boolean(payment && order) && paymentPaid !== orderPaid;
  const amountsDiffer = Boolean(
    payment && order && order.amount_minor > 0 && payment.amount_minor !== order.amount_minor,
  );
  const currenciesDiffer = Boolean(
    payment && order && order.currency && payment.currency !== order.currency,
  );
  const inSync = !statusesDiffer && !amountsDiffer && !currenciesDiffer;

  const alreadyResolved = ['RESOLVED', 'CLOSED', 'REJECTED'].includes(
    caseData.status.toUpperCase(),
  );
  const canReconcile =
    caseData.available_actions.includes('RECONCILE_MERCHANT_ORDER') && !alreadyResolved;
  const disabledReason = alreadyResolved
    ? 'This case is already resolved.'
    : 'Reconciling isn’t available for this case.';

  const evidenceCount = evidence.length || investigation?.evidence_ids.length || 0;
  const validation = formatValidation(investigation?.validation_status);
  const decisionMeta = formatDecision(decision?.outcome);
  const matchedFinding = decision?.findings?.find((finding) => finding.passed) ?? decision?.findings?.[0];
  const decisionReason =
    matchedFinding?.message ||
    (decision
      ? `PolicyGuard ${decisionMeta.label.toLowerCase()} this action under ${decision.policy_revision}.`
      : '');

  const timelineEntries: TimelineEntry[] =
    caseData.activity?.map((item) => ({
      id: item.id,
      time: item.created_at,
      title: formatEventType(item.event_type),
      actor: formatActor(item.actor_type),
      customerVisible: isCustomerVisible(item.visibility),
      detail:
        item.data && typeof item.data.message === 'string' ? item.data.message : undefined,
    })) ?? [];

  return (
    <div className="space-y-5">
      <div className="page-sticky-header">
        <Link
          to="/merchant/cases"
          className="inline-flex items-center gap-1.5 text-meta font-medium text-blue hover:underline"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" />
          Back to cases
        </Link>
        <div className="mt-2 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="text-title font-extrabold text-ink">Case {caseData.case_number}</h1>
              <span key={caseData.status} className="animate-fade-in" aria-live="polite">
                <StatusPill meta={statusMeta(caseData.status)} />
              </span>
            </div>
            <p className="mt-0.5 text-meta text-muted">
              Opened{' '}
              <Tooltip content={formatAbsolute(caseData.created_at)}>
                <time dateTime={caseData.created_at}>{formatTime(caseData.created_at)}</time>
              </Tooltip>{' '}
              for {customerName}
            </p>
          </div>

          <div className="flex shrink-0 items-center gap-2">
            {canReconcile ? (
              <Button
                variant="primary"
                onClick={() => {
                  setReconcileError(null);
                  setConfirmOpen(true);
                }}
                isLoading={reconciling}
                loadingText="Reconciling…"
                icon={<RefreshCw className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
              >
                Reconcile order
              </Button>
            ) : (
              <Tooltip content={disabledReason} side="bottom">
                <span className="inline-flex">
                  <Button
                    variant="primary"
                    disabled
                    icon={<RefreshCw className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
                  >
                    Reconcile order
                  </Button>
                </span>
              </Tooltip>
            )}

            <div className="relative" ref={menuRef}>
              <IconButton
                label="More case actions"
                variant="outline"
                onClick={() => setMenuOpen((value) => !value)}
                aria-haspopup="menu"
                aria-expanded={menuOpen}
              >
                <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
              </IconButton>
              {menuOpen && (
                <div
                  role="menu"
                  className="absolute right-0 top-full z-50 mt-2 w-60 animate-slide-up rounded border-2 border-line bg-surface p-1.5 shadow-hard-lg"
                >
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => void copyText(window.location.href, 'Case link copied')}
                    className="flex w-full items-center gap-2 rounded px-3 py-2.5 text-left text-body font-medium text-ink transition-colors hover:bg-surface-alt"
                  >
                    <Link2 className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2} />
                    Copy case link
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => void copyText(caseData.id, 'Case ID copied')}
                    className="flex w-full items-center gap-2 rounded px-3 py-2.5 text-left text-body font-medium text-ink transition-colors hover:bg-surface-alt"
                  >
                    <Hash className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2} />
                    Copy case ID
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="page-grid">
        <div className="page-grid-main space-y-5">
          {/* Payment vs order */}
          <section className={cn(PANEL, 'overflow-hidden p-0')}>
            <div className="rail-head flex flex-wrap items-center justify-between gap-2">
              <h2 className="text-section font-bold">Payment vs order</h2>
              <StatusPill
                meta={{
                  label: inSync ? 'In sync' : 'Out of sync',
                  tone: inSync ? 'success' : 'warning',
                }}
              />
            </div>
            <div className="cq">
              <div className="split-panel">
                <div className="p-5">
                  <p className="section-tag text-muted">PayPal</p>
                  <p className="mt-1 text-hero font-extrabold tabular-nums text-ink">
                    {payment ? formatMinorUnits(payment.amount_minor, payment.currency) : '—'}
                  </p>
                  <div className="mt-2">
                    <StatusPill meta={statusMeta(payment?.status)} />
                  </div>

                  <dl className="mt-4 space-y-2.5 border-t-2 border-line pt-3">
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-meta text-muted">Provider order</dt>
                      <dd>
                        {provider?.order_id ? (
                          <CopyChip value={provider.order_id} title="Copy provider order ID" />
                        ) : (
                          <span className="text-meta text-muted">—</span>
                        )}
                      </dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-meta text-muted">Capture</dt>
                      <dd>
                        {provider?.capture_id ? (
                          <CopyChip value={provider.capture_id} title="Copy capture ID" />
                        ) : (
                          <span className="text-meta text-muted">—</span>
                        )}
                      </dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-meta text-muted">Captured</dt>
                      <dd className="font-mono text-[12px] text-ink">
                        {provider?.fetched_at ? formatAbsolute(provider.fetched_at) : '—'}
                      </dd>
                    </div>
                  </dl>
                </div>

                <div className="p-5">
                  <p className="section-tag text-muted">Your store</p>
                  <p className="mt-1 break-all font-mono text-[18px] font-bold text-ink sm:text-[20px]">
                    {order?.ref ?? '—'}
                  </p>
                  <div
                    className={cn(
                      'mt-2 w-fit',
                      statusesDiffer && 'rounded-pill border-2 border-line bg-warning px-2 py-1',
                    )}
                  >
                    <StatusPill meta={statusMeta(order?.status)} />
                  </div>

                  <dl className="mt-4 space-y-2.5 border-t-2 border-line pt-3">
                    <div
                      className={cn(
                        'flex items-center justify-between gap-3',
                        amountsDiffer &&
                          'rounded-pill border-2 border-line bg-warning px-2 py-1 text-[color:var(--warning-ink)]',
                      )}
                    >
                      <dt className={cn('text-meta', amountsDiffer ? 'font-bold' : 'text-muted')}>
                        Order total
                      </dt>
                      <dd className="text-meta font-bold tabular-nums text-ink">
                        {order && order.amount_minor > 0
                          ? formatMinorUnits(order.amount_minor, order.currency)
                          : '—'}
                      </dd>
                    </div>
                    <div className="flex items-center justify-between gap-3">
                      <dt className="text-meta text-muted">Last updated</dt>
                      <dd className="font-mono text-[12px] text-ink">
                        {order?.fetched_at ? (
                          <Tooltip content={formatAbsolute(order.fetched_at)} side="left">
                            <time dateTime={order.fetched_at}>{formatTime(order.fetched_at)}</time>
                          </Tooltip>
                        ) : (
                          '—'
                        )}
                      </dd>
                    </div>
                  </dl>
                </div>
              </div>
            </div>
          </section>

          {/* AI investigation */}
          <section className={cn(PANEL, 'overflow-hidden p-0')}>
            <div className="rail-head flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <Sparkles className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
                <h2 className="text-section font-bold">AI investigation</h2>
                <Tag tone="neutral">Advisory</Tag>
              </div>
              {investigation && <StatusPill meta={formatRisk(investigation.suggested_severity)} />}
            </div>

            {investigation ? (
              <div className="p-5">
                <p className="text-body font-bold text-ink">
                  {formatCategory(investigation.category)}
                </p>
                <p className="mt-1.5 max-w-prose text-body text-muted">
                  {investigation.root_cause_hypothesis}
                </p>

                {evidenceCount > 0 && (
                  <div className="mt-4 border-t-2 border-line pt-3">
                    <Disclosure
                      title="Evidence"
                      meta={`${evidenceCount} ${evidenceCount === 1 ? 'item' : 'items'}, ${validation.label.toLowerCase()}`}
                    >
                      <ul className="space-y-1.5">
                        {(evidence.length > 0
                          ? evidence
                          : (investigation.evidence_ids ?? []).map((evidenceId) => ({
                              id: evidenceId,
                              evidence_type: 'evidence',
                              source_system: '',
                              source_ref: '',
                              fetched_at: '',
                            }))
                        ).map((item) => (
                          <li
                            key={item.id}
                            className="flex items-center justify-between gap-3 rounded-pill border-2 border-line bg-surface-alt px-2.5 py-1.5"
                          >
                            <span className="min-w-0 truncate text-meta font-medium text-ink">
                              {sentenceCase(item.evidence_type)}
                              {item.source_system && (
                                <span className="ml-1.5 font-mono text-[11px] text-muted">
                                  {sentenceCase(item.source_system)}
                                </span>
                              )}
                            </span>
                            <CopyChip value={item.id} title="Copy evidence ID" />
                          </li>
                        ))}
                      </ul>
                    </Disclosure>
                  </div>
                )}

                <p className="mt-4 text-[12px] leading-5 text-muted">
                  AI suggestions are advisory. Reconcile to check this case against your policy.
                </p>
              </div>
            ) : (
              <p className="p-5 text-body text-muted">
                No AI investigation has been recorded for this case yet. You can still reconcile —
                PolicyGuard checks every action.
              </p>
            )}
          </section>

          {/* PolicyGuard */}
          <section className={cn(PANEL, 'overflow-hidden p-0')}>
            <div className="rail-head flex flex-wrap items-center justify-between gap-2">
              <div className="flex items-center gap-2">
                <ShieldCheck className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
                <h2 className="text-section font-bold">PolicyGuard decision</h2>
              </div>
              {decision && (
                <span key={decision.id} className="animate-fade-in">
                  <StatusPill meta={decisionMeta} />
                </span>
              )}
            </div>

            <div className="p-5">
              {reconcileError && (
                <ErrorNotice
                  className="mb-4"
                  title="We couldn’t reconcile this order"
                  description={reconcileError}
                  onRetry={() => void performReconcile()}
                />
              )}

              {decision ? (
                <div>
                  <div
                    className={cn(
                      'rounded border-2 border-line p-4 shadow-hard',
                      TONE_SURFACE[decisionMeta.tone],
                    )}
                  >
                    <p className="text-[11px] font-bold uppercase tracking-[0.04em] opacity-80">
                      Outcome
                    </p>
                    <p className="text-section font-extrabold">{decisionMeta.label}</p>
                    <p className="mt-1 text-body font-medium">{decisionReason}</p>
                  </div>

                  <dl className="mt-4 flex flex-wrap gap-x-8 gap-y-2 border-t-2 border-line pt-3">
                    <div>
                      <dt className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">
                        Rule
                      </dt>
                      <dd className="font-mono text-[12px] text-ink">
                        {matchedFinding?.rule_id || matchedFinding?.code || '—'}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">
                        Policy revision
                      </dt>
                      <dd className="font-mono text-[12px] text-ink">
                        {decision.policy_revision || '—'}
                      </dd>
                    </div>
                    {decision.created_at && (
                      <div>
                        <dt className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">
                          Decided
                        </dt>
                        <dd className="font-mono text-[12px] text-ink">
                          <Tooltip content={formatAbsolute(decision.created_at)} side="left">
                            <time dateTime={decision.created_at}>
                              {formatTime(decision.created_at)}
                            </time>
                          </Tooltip>
                        </dd>
                      </div>
                    )}
                  </dl>

                  {decision.findings?.length > 0 && (
                    <div className="mt-4 border-t-2 border-line pt-3">
                      <p className="text-meta font-bold text-ink">Checks</p>
                      <ul className="mt-2 space-y-1.5">
                        {decision.findings.map((finding, index) => (
                          <li
                            key={`${finding.rule_id ?? finding.code ?? 'check'}-${index}`}
                            className="flex items-start gap-2 text-meta"
                          >
                            {finding.passed === false ? (
                              <X className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[color:var(--danger-text)]" aria-hidden="true" strokeWidth={3} />
                            ) : (
                              <Check
                                className="mt-0.5 h-3.5 w-3.5 shrink-0 text-ink"
                                aria-hidden="true"
                                strokeWidth={3}
                              />
                            )}
                            <span className="min-w-0 flex-1 text-ink">
                              {finding.message || sentenceCase(finding.code || finding.rule_id || 'Check')}
                            </span>
                            {(finding.rule_id || finding.code) && (
                              <span className="hidden shrink-0 font-mono text-[11px] text-muted sm:inline">
                                {finding.rule_id || finding.code}
                              </span>
                            )}
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}

                  {(caseData.pending_approvals ?? []).length > 0 && (
                    <div className="mt-4 rounded border-2 border-line bg-warning p-3 text-[color:var(--warning-ink)]">
                      <p className="text-meta font-bold uppercase tracking-[0.04em]">Approval required</p>
                      <ul className="mt-1 space-y-1">
                        {caseData.pending_approvals.map((approval) => (
                          <li key={approval.id} className="text-meta font-medium">
                            {sentenceCase(approval.action_type)}
                            <span className="ml-1.5 font-mono text-[11px]">
                              expires {formatTime(approval.expires_at)}
                            </span>
                          </li>
                        ))}
                      </ul>
                      <p className="mt-1.5 text-[11px] font-medium">
                        Requests are read-only here. Use your approvals workflow to decide.
                      </p>
                    </div>
                  )}
                </div>
              ) : (
                <div className="rounded border-2 border-dashed border-line p-4">
                  <p className="text-body font-medium text-ink">No policy decision yet.</p>
                  {canReconcile ? (
                    <button
                      type="button"
                      onClick={() => {
                        setReconcileError(null);
                        setConfirmOpen(true);
                      }}
                      className="mt-1 text-body font-bold text-blue underline-offset-4 hover:underline"
                    >
                      Reconcile to check this case against your policy.
                    </button>
                  ) : (
                    <p className="mt-1 text-body text-muted">
                      Reconciling is not available for this case.
                    </p>
                  )}
                </div>
              )}
            </div>
          </section>

          {/* Incident */}
          {caseData.incident && (
            <section className={cn(PANEL, 'border-l-[6px] border-l-info p-5')}>
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Layers className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2.5} />
                    <h2 className="text-body font-bold text-ink">Grouped with other cases</h2>
                    <StatusPill meta={formatRisk(caseData.incident.severity)} size="sm" />
                  </div>
                  <p className="mt-2 text-body text-ink">{caseData.incident.summary}</p>
                  <p className="mt-1 text-[12px] text-muted">
                    <span className="font-mono">{caseData.incident.number}</span>
                    {' · '}
                    {caseData.incident.open_case_count} of {caseData.incident.case_count} cases still open
                  </p>
                </div>
                <Link
                  to={`/merchant/incidents/${caseData.incident.id}`}
                  className="btn btn-lift inline-flex h-9 items-center justify-center gap-2 rounded bg-yellow px-3 text-meta font-extrabold text-[color:var(--text-on-yellow)] hover:bg-yellow-hover"
                >
                  Open incident
                </Link>
              </div>
              <p className="mt-3 flex items-start gap-2 text-[12px] text-muted">
                <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" strokeWidth={2.5} />
                <span>{caseData.incident.next_step}</span>
              </p>
            </section>
          )}

          {/* Drafted replies */}
          <section className={cn(PANEL, 'p-5')}>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <Sparkles className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2.5} />
                <h2 className="text-body font-bold text-ink">Drafted reply</h2>
              </div>
              {(caseData.drafts ?? []).length === 0 ? (
                <Button
                  variant="outline"
                  size="sm"
                  isLoading={drafting}
                  loadingText="Drafting…"
                  onClick={() => void draftReply()}
                  icon={<Sparkles className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
                >
                  Draft a reply
                </Button>
              ) : null}
            </div>
            {(caseData.drafts ?? []).length === 0 ? (
              <p className="mt-2 text-[12px] text-muted">
                The assistant writes replies from the verified facts on this case. Nothing reaches the customer
                until you approve it.
              </p>
            ) : null}
            <ul className="mt-3 space-y-3">
              {(caseData.drafts ?? []).map((draft) => (
                <li key={draft.id} className="rounded border-2 border-line bg-surface-alt p-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusPill
                      meta={{ label: draft.status.toLowerCase(), tone: 'neutral' }}
                      size="sm"
                    />
                    {draft.guardrail_flag ? (
                      <StatusPill meta={{ label: 'Merchant decision needed', tone: 'warning' }} size="sm" />
                    ) : null}
                    <div className="ml-auto flex items-center gap-2">
                      {draft.status === 'DRAFTED' ? (
                        <Button
                          variant="secondary"
                          size="sm"
                          isLoading={draftBusy === `approve-${draft.id}`}
                          loadingText="Approving…"
                          onClick={() => void approveDraft(draft.id)}
                          icon={<Check className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
                        >
                          Approve
                        </Button>
                      ) : null}
                      {draft.status === 'APPROVED' ? (
                        <Button
                          variant="primary"
                          size="sm"
                          isLoading={draftBusy === `send-${draft.id}`}
                          loadingText="Sending…"
                          onClick={() => void sendDraft(draft.id)}
                          icon={<Send className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
                        >
                          Send
                        </Button>
                      ) : null}
                    </div>
                  </div>
                  <p className="mt-2 whitespace-pre-wrap text-body text-ink">{draft.body}</p>
                  {draft.incident_id ? (
                    <Link
                      to={`/merchant/incidents/${draft.incident_id}`}
                      className="mt-2 inline-block text-[12px] font-bold text-blue underline underline-offset-4"
                    >
                      Review in the incident inbox
                    </Link>
                  ) : null}
                </li>
              ))}
            </ul>
          </section>

          {/* Conversation */}
          <section className={cn(PANEL, 'p-6')}>
            <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
              <h2 className="text-section font-bold text-ink">Conversation</h2>
              <p className="text-[12px] text-muted">Messages are shared with the customer.</p>
            </div>
            <Conversation
              messages={caseData.messages ?? []}
              perspective="merchant"
              onSend={sendMessage}
              emptyText="No messages yet. Reply to keep the customer informed."
            />
          </section>
        </div>

        <aside className="page-grid-side space-y-5">
          <section className={cn(PANEL, 'p-5')}>
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <History className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2} />
                <h2 className="text-body font-bold text-ink">Activity</h2>
              </div>
              <label className="flex min-h-[32px] cursor-pointer items-center gap-1.5 text-[11px] font-bold text-muted">
                <input
                  type="checkbox"
                  checked={showMerchantOnly}
                  onChange={(event) => setShowMerchantOnly(event.target.checked)}
                  className="h-4 w-4 accent-[color:var(--brand-blue)]"
                />
                Show merchant-only
              </label>
            </div>
            <div className="mt-4">
              <Timeline
                entries={timelineEntries}
                showMerchantOnly={showMerchantOnly}
                emptyText="No activity has been recorded yet."
              />
            </div>
          </section>

          <PermissionsCard />

          {caseData.anomaly && (
            <section className={cn(PANEL, 'p-5')}>
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-body font-bold text-ink">Operational anomaly</h2>
                <StatusPill
                  meta={anomalyMeta(caseData.anomaly.score, caseData.anomaly.threshold)}
                  size="sm"
                />
              </div>
              <p className="mt-3 text-hero font-extrabold tabular-nums text-ink">
                {caseData.anomaly.score.toFixed(2)}
              </p>
              <p className="font-mono text-[11px] text-muted">
                Threshold {caseData.anomaly.threshold.toFixed(2)}
              </p>
              <dl className="mt-3 space-y-1.5 border-t-2 border-line pt-3 text-meta">
                {caseData.anomaly.features?.webhook_retry_count != null && (
                  <div className="flex items-center justify-between gap-3">
                    <dt className="text-muted">Webhook retries</dt>
                    <dd className="tabular-nums text-ink">
                      {caseData.anomaly.features.webhook_retry_count}
                    </dd>
                  </div>
                )}
                {caseData.anomaly.features?.handler_error_rate != null && (
                  <div className="flex items-center justify-between gap-3">
                    <dt className="text-muted">Handler error rate</dt>
                    <dd className="tabular-nums text-ink">
                      {(caseData.anomaly.features.handler_error_rate * 100).toFixed(1)}%
                    </dd>
                  </div>
                )}
              </dl>
              <p className="mt-3 text-[11px] leading-5 text-muted">
                An elevated score can require admin approval. It can never allow a blocked action.
                Model {caseData.anomaly.model_id}
                {caseData.anomaly.model_version ? ` (${caseData.anomaly.model_version})` : ''}.
              </p>
            </section>
          )}
        </aside>
      </div>

      <ConfirmDialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => void performReconcile()}
        title="Reconcile this order?"
        description={
          <>
            PolicyGuard will check this change against your policy. If approved, order{' '}
            <span className="font-mono text-ink">{order?.ref ?? '—'}</span> will be marked as paid
            for <span className="font-medium text-ink">{payment ? formatMinorUnits(payment.amount_minor, payment.currency) : '—'}</span>.
          </>
        }
        confirmLabel="Confirm reconcile"
        loading={reconciling}
      />
    </div>
  );
}
