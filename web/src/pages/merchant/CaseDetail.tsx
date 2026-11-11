import { useCallback, useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  ArrowLeft,
  Check,
  Hash,
  History,
  Link2,
  MoreHorizontal,
  RefreshCw,
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
import { StatusPill, Tag } from '../../components/ui/StatusPill';
import { CopyChip } from '../../components/ui/CopyChip';
import { Tooltip } from '../../components/ui/Tooltip';
import { Disclosure } from '../../components/ui/Disclosure';
import { ConfirmDialog } from '../../components/ui/Dialog';
import { ErrorNotice, Skeleton } from '../../components/ui/States';
import { Timeline, type TimelineEntry } from '../../components/ui/Timeline';
import { Conversation } from '../../components/Conversation';

const PANEL = 'rounded-lg border border-line bg-surface';

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
    <section ref={ref} className={cn(PANEL, 'relative p-4')}>
      <div className="flex items-center justify-between gap-3">
        <div>
          <h2 className="text-body font-semibold text-ink">Your access</h2>
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
        <div className="absolute right-4 top-full z-30 mt-1 w-72 animate-slide-up rounded-lg border border-line bg-surface p-4 shadow-pop">
          <p className="text-[11px] font-semibold text-success">Can</p>
          <ul className="mt-2 space-y-1.5">
            {[
              'Reconcile orders',
              'Approve requests from other admins',
              'See the risk score for this case',
            ].map((item) => (
              <li key={item} className="flex items-start gap-2 text-meta text-ink">
                <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" aria-hidden="true" />
                {item}
              </li>
            ))}
          </ul>
          <p className="mt-4 text-[11px] font-semibold text-muted">Cannot</p>
          <ul className="mt-2 space-y-1.5">
            {['Edit PolicyGuard rules', 'See cases from other merchants'].map((item) => (
              <li key={item} className="flex items-start gap-2 text-meta text-ink">
                <X className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted" aria-hidden="true" />
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
        <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
          <div className="space-y-5">
            <Skeleton className="h-64 w-full rounded-lg" />
            <Skeleton className="h-56 w-full rounded-lg" />
            <Skeleton className="h-40 w-full rounded-lg" />
          </div>
          <div className="space-y-5">
            <Skeleton className="h-80 w-full rounded-lg" />
            <Skeleton className="h-24 w-full rounded-lg" />
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
      <div className="sticky top-16 z-30 -mx-4 -mt-6 border-b border-line bg-bg px-4 py-3 sm:-mx-6 sm:px-6 lg:-mx-8 lg:px-8">
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
              <h1 className="text-title font-semibold text-ink">Case {caseData.case_number}</h1>
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
                  className="absolute right-0 top-full z-50 mt-1 w-56 animate-slide-up rounded-lg border border-line bg-surface p-1.5 shadow-pop"
                >
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => void copyText(window.location.href, 'Case link copied')}
                    className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-left text-body text-ink transition-colors hover:bg-surface-muted"
                  >
                    <Link2 className="h-4 w-4 text-muted" aria-hidden="true" strokeWidth={1.75} />
                    Copy case link
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => void copyText(caseData.id, 'Case ID copied')}
                    className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-left text-body text-ink transition-colors hover:bg-surface-muted"
                  >
                    <Hash className="h-4 w-4 text-muted" aria-hidden="true" strokeWidth={1.75} />
                    Copy case ID
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0 space-y-5">
          {/* Payment vs order */}
          <section className={cn(PANEL, 'p-6')}>
            <div className="mb-4 flex justify-center">
              <StatusPill
                meta={{
                  label: inSync ? 'In sync' : 'Out of sync',
                  tone: inSync ? 'success' : 'warning',
                }}
              />
            </div>
            <div className="grid gap-6 sm:grid-cols-2 sm:gap-0">
              <div className="sm:pr-6">
                <p className="text-meta text-muted">PayPal</p>
                <p className="mt-1 text-hero font-semibold tabular-nums text-ink">
                  {payment ? formatMinorUnits(payment.amount_minor, payment.currency) : '—'}
                </p>
                <div className="mt-2">
                  <StatusPill meta={statusMeta(payment?.status)} />
                </div>

                <dl className="mt-4 space-y-2.5 border-t border-line pt-3">
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
                    <dd className="text-meta text-ink">
                      {provider?.fetched_at ? formatAbsolute(provider.fetched_at) : '—'}
                    </dd>
                  </div>
                </dl>
              </div>

              <div className="border-t border-line pt-6 sm:border-l sm:border-t-0 sm:pl-6 sm:pt-0">
                <p className="text-meta text-muted">Your store</p>
                <p className="mt-1 break-all font-mono text-[18px] font-semibold text-ink sm:text-[20px]">
                  {order?.ref ?? '—'}
                </p>
                <div
                  className={cn(
                    'mt-2 w-fit rounded-sm',
                    statusesDiffer && 'bg-warning-bg px-2 py-1',
                  )}
                >
                  <StatusPill meta={statusMeta(order?.status)} />
                </div>

                <dl className="mt-4 space-y-2.5 border-t border-line pt-3">
                  <div
                    className={cn(
                      'flex items-center justify-between gap-3 rounded-sm',
                      amountsDiffer && 'bg-warning-bg px-2 py-1',
                    )}
                  >
                    <dt className="text-meta text-muted">Order total</dt>
                    <dd className="text-meta font-medium tabular-nums text-ink">
                      {order && order.amount_minor > 0
                        ? formatMinorUnits(order.amount_minor, order.currency)
                        : '—'}
                    </dd>
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <dt className="text-meta text-muted">Last updated</dt>
                    <dd className="text-meta text-ink">
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
          </section>

          {/* AI investigation */}
          <section className={cn(PANEL, 'p-6')}>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="flex items-center gap-2">
                <Sparkles className="h-4 w-4 text-info" aria-hidden="true" strokeWidth={1.75} />
                <h2 className="text-section font-semibold text-ink">AI investigation</h2>
                <Tag tone="neutral">Advisory</Tag>
              </div>
              {investigation && <StatusPill meta={formatRisk(investigation.suggested_severity)} />}
            </div>

            {investigation ? (
              <div className="mt-4">
                <p className="text-body font-medium text-ink">
                  {formatCategory(investigation.category)}
                </p>
                <p className="mt-1.5 max-w-prose text-body text-muted">
                  {investigation.root_cause_hypothesis}
                </p>

                {evidenceCount > 0 && (
                  <div className="mt-4 border-t border-line pt-3">
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
                            className="flex items-center justify-between gap-3 rounded-sm bg-surface-muted px-2.5 py-1.5"
                          >
                            <span className="min-w-0 truncate text-meta text-ink">
                              {sentenceCase(item.evidence_type)}
                              {item.source_system && (
                                <span className="ml-1.5 text-[11px] text-muted">
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
              <p className="mt-4 text-body text-muted">
                No AI investigation has been recorded for this case yet. You can still reconcile —
                PolicyGuard checks every action.
              </p>
            )}
          </section>

          {/* PolicyGuard */}
          <section className={cn(PANEL, 'p-6')}>
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="flex items-center gap-2">
                <ShieldCheck className="h-4 w-4 text-success" aria-hidden="true" strokeWidth={1.75} />
                <h2 className="text-section font-semibold text-ink">PolicyGuard decision</h2>
              </div>
              {decision && (
                <span key={decision.id} className="animate-fade-in">
                  <StatusPill
                    meta={decisionMeta}
                    className="h-8 px-3 text-body"
                  />
                </span>
              )}
            </div>

            {reconcileError && (
              <ErrorNotice
                className="mt-4"
                title="We couldn’t reconcile this order"
                description={reconcileError}
                onRetry={() => void performReconcile()}
              />
            )}

            {decision ? (
              <div className="mt-4">
                <p className="max-w-prose text-body text-ink">{decisionReason}</p>
                <dl className="mt-3 flex flex-wrap gap-x-8 gap-y-2 border-t border-line pt-3">
                  <div>
                    <dt className="text-[11px] text-muted">Rule</dt>
                    <dd className="font-mono text-[12px] text-ink">
                      {matchedFinding?.rule_id || matchedFinding?.code || '—'}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-[11px] text-muted">Policy revision</dt>
                    <dd className="font-mono text-[12px] text-ink">
                      {decision.policy_revision || '—'}
                    </dd>
                  </div>
                  {decision.created_at && (
                    <div>
                      <dt className="text-[11px] text-muted">Decided</dt>
                      <dd className="text-[12px] text-ink">
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
                  <div className="mt-4 border-t border-line pt-3">
                    <p className="text-meta font-medium text-ink">Checks</p>
                    <ul className="mt-2 space-y-1.5">
                      {decision.findings.map((finding, index) => (
                        <li
                          key={`${finding.rule_id ?? finding.code ?? 'check'}-${index}`}
                          className="flex items-start gap-2 text-meta"
                        >
                          {finding.passed === false ? (
                            <X className="mt-0.5 h-3.5 w-3.5 shrink-0 text-danger" aria-hidden="true" />
                          ) : (
                            <Check
                              className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success"
                              aria-hidden="true"
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
                  <div className="mt-4 rounded-md bg-warning-bg p-3">
                    <p className="text-meta font-medium text-warning">Approval required</p>
                    <ul className="mt-1 space-y-1">
                      {caseData.pending_approvals.map((approval) => (
                        <li key={approval.id} className="text-meta text-ink">
                          {sentenceCase(approval.action_type)}
                          <span className="ml-1.5 text-[11px] text-muted">
                            expires {formatTime(approval.expires_at)}
                          </span>
                        </li>
                      ))}
                    </ul>
                    <p className="mt-1.5 text-[11px] text-muted">
                      Requests are read-only here. Use your approvals workflow to decide.
                    </p>
                  </div>
                )}
              </div>
            ) : (
              <p className="mt-4 text-body text-muted">
                No policy decision yet.{' '}
                {canReconcile ? (
                  <button
                    type="button"
                    onClick={() => {
                      setReconcileError(null);
                      setConfirmOpen(true);
                    }}
                    className="font-medium text-blue hover:underline"
                  >
                    Reconcile to check this case against your policy.
                  </button>
                ) : (
                  'Reconciling is not available for this case.'
                )}
              </p>
            )}
          </section>

          {/* Conversation */}
          <section className={cn(PANEL, 'p-6')}>
            <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
              <h2 className="text-section font-semibold text-ink">Conversation</h2>
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

        <aside className="min-w-0 space-y-5">
          <section className={cn(PANEL, 'p-4')}>
            <div className="flex items-center justify-between gap-3">
              <div className="flex items-center gap-2">
                <History className="h-4 w-4 text-muted" aria-hidden="true" strokeWidth={1.75} />
                <h2 className="text-body font-semibold text-ink">Activity</h2>
              </div>
              <label className="flex cursor-pointer items-center gap-1.5 text-[11px] text-muted">
                <input
                  type="checkbox"
                  checked={showMerchantOnly}
                  onChange={(event) => setShowMerchantOnly(event.target.checked)}
                  className="h-3.5 w-3.5 accent-blue"
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
            <section className={cn(PANEL, 'p-4')}>
              <div className="flex items-center justify-between gap-3">
                <h2 className="text-body font-semibold text-ink">Operational anomaly</h2>
                <StatusPill
                  meta={anomalyMeta(caseData.anomaly.score, caseData.anomaly.threshold)}
                  size="sm"
                />
              </div>
              <p className="mt-3 text-hero font-semibold tabular-nums text-ink">
                {caseData.anomaly.score.toFixed(2)}
              </p>
              <p className="text-[11px] text-muted">
                Threshold {caseData.anomaly.threshold.toFixed(2)}
              </p>
              <dl className="mt-3 space-y-1.5 border-t border-line pt-3 text-meta">
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
