import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import {
  AlertTriangle,
  ArrowLeft,
  Check,
  CheckCheck,
  FileText,
  Link2Off,
  Pencil,
  Send,
  ShieldCheck,
  Sparkles,
  Trash2,
  Wrench,
} from 'lucide-react';
import { toast } from 'sonner';
import { apiClient, type Incident, type MessageDraft } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { cn } from '../../lib/cn';
import {
  apiErrorMessage,
  formatAbsolute,
  formatDraftType,
  formatMinorUnits,
  formatRisk,
  formatTime,
  statusMeta,
} from '../../lib/format';
import { PageHeader } from '../../components/ui/Panel';
import { Button, IconButton } from '../../components/ui/Button';
import { StatusPill, Tag } from '../../components/ui/StatusPill';
import { Tooltip } from '../../components/ui/Tooltip';
import { EmptyState, ErrorNotice, Skeleton } from '../../components/ui/States';

function draftTone(status: string): 'neutral' | 'warning' | 'success' | 'info' {
  switch (status) {
    case 'APPROVED':
      return 'info';
    case 'SENT':
      return 'success';
    case 'DRAFTED':
      return 'warning';
    default:
      return 'neutral';
  }
}

export function MerchantIncidentDetail() {
  const { id } = useParams<{ id: string }>();
  const [incident, setIncident] = useState<Incident | null>(null);
  const [drafts, setDrafts] = useState<MessageDraft[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [editing, setEditing] = useState<string | null>(null);
  const [editBody, setEditBody] = useState('');
  const { logout } = useAuthStore();
  const navigate = useNavigate();

  const load = useCallback(async () => {
    if (!id) return;
    setLoading(true);
    try {
      const result = await apiClient.getIncident(id);
      setIncident(result.incident);
      setDrafts(result.drafts);
      setError(null);
    } catch (err) {
      const raw = err instanceof Error ? err.message : '';
      setError(apiErrorMessage(err, 'We couldn’t load this incident.'));
      if (raw.includes('401')) {
        logout();
        navigate('/login');
      }
    } finally {
      setLoading(false);
    }
  }, [id, logout, navigate]);

  useEffect(() => {
    void load();
  }, [load]);

  const run = async (key: string, action: () => Promise<void>) => {
    setBusy(key);
    try {
      await action();
      await load();
    } catch (err) {
      toast.error(apiErrorMessage(err, 'That action didn’t go through.'));
    } finally {
      setBusy(null);
    }
  };

  const draftsByCase = useMemo(() => {
    const map = new Map<string, MessageDraft>();
    drafts.forEach((draft) => {
      if (!map.has(draft.case_id)) map.set(draft.case_id, draft);
    });
    return map;
  }, [drafts]);

  const pendingCount = drafts.filter((d) => d.status === 'DRAFTED').length;
  const approvedCount = drafts.filter((d) => d.status === 'APPROVED').length;

  if (loading) {
    return (
      <div className="page-fill gap-6">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-10 w-2/3" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  if (error || !incident) {
    return (
      <div className="page-fill gap-6">
        <PageHeader title="Incident" description="A grouped root cause across customer cases." />
        <ErrorNotice title="We couldn’t load this incident" description={error ?? ''} onRetry={() => void load()} />
      </div>
    );
  }

  return (
    <div className="page-fill">
      <div className="list-layout flex-1">
        <div className="flex min-w-0 flex-col gap-5">
          <Link
            to="/merchant/incidents"
            className="inline-flex w-fit items-center gap-1.5 text-meta font-bold text-blue underline underline-offset-4"
          >
            <ArrowLeft className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
            Back to the inbox
          </Link>

          <PageHeader
            title={`${incident.number} · ${incident.root_cause_summary}`}
            description={incident.ai_summary}
            actions={
              <div className="flex flex-wrap items-center gap-2">
                <StatusPill meta={formatRisk(incident.severity)} />
                <StatusPill meta={statusMeta(incident.status)} withDot={false} />
              </div>
            }
          />

          <section className="grid gap-4 md:grid-cols-3">
            <div className="rounded border-2 border-line bg-surface p-4 shadow-hard">
              <p className="meta-label">Cases affected</p>
              <p className="text-[28px] font-extrabold leading-8 text-ink">
                {incident.case_count}
              </p>
              <p className="text-meta text-muted">{incident.open_case_count} still open</p>
            </div>
            <div className="rounded border-2 border-line bg-surface p-4 shadow-hard">
              <p className="meta-label">Amount in question</p>
              <p className="text-[22px] font-extrabold leading-8 text-ink">
                {formatMinorUnits(incident.affected_amount_minor, incident.currency)}
              </p>
              <p className="text-meta text-muted">Across every linked case</p>
            </div>
            <div className="rounded border-2 border-line bg-surface p-4 shadow-hard">
              <p className="meta-label">Grouping confidence</p>
              <p className="text-[28px] font-extrabold leading-8 text-ink">
                {Math.round(incident.confidence * 100)}%
              </p>
              <p className="text-meta text-muted">Verified facts matched</p>
            </div>
          </section>

          <section className="rounded border-2 border-line bg-surface p-4 shadow-hard">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <h2 className="text-section font-bold text-ink">Suggested next step</h2>
                <p className="mt-1 text-body text-muted">{incident.next_step}</p>
              </div>
              <Button
                variant="secondary"
                size="sm"
                isLoading={busy === 'fanout'}
                loadingText="Reconciling…"
                onClick={() =>
                  void run('fanout', async () => {
                    const result = await apiClient.fanOutIncident(incident.id);
                    const allowed = result.results.filter(
                      (r) => r.outcome === 'ALLOW' || r.status === 'COMPLETED',
                    ).length;
                    const blocked = result.results.filter((r) => r.outcome === 'BLOCK').length;
                    toast.success('Fan-out complete', {
                      description: `${allowed} reconciled, ${blocked} blocked by policy. Each case was checked on its own.`,
                    });
                  })
                }
                icon={<Wrench className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
              >
                Fix cause and reconcile all
              </Button>
            </div>
            <p className="mt-3 flex items-start gap-2 text-meta text-muted">
              <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" strokeWidth={2.5} />
              <span>
                Every linked case is proposed separately and PolicyGuard decides each one on its own facts. A
                mismatch in one case never reconciles the others.
              </span>
            </p>
          </section>

          <section className="rounded border-2 border-line bg-surface shadow-hard">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b-2 border-line bg-surface-alt px-4 py-3">
              <div className="min-w-0">
                <h2 className="text-section font-bold text-ink">Drafted replies</h2>
                <p className="mt-0.5 text-meta text-muted">
                  Written from verified case facts only. Every reply is labeled AI-assisted and recorded on the
                  audit timeline.
                </p>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  isLoading={busy === 'draft'}
                  loadingText="Drafting…"
                  disabled={drafts.length > 0 && pendingCount === 0}
                  onClick={() =>
                    void run('draft', async () => {
                      await apiClient.draftIncidentReplies(incident.id);
                      toast.success('Replies drafted', {
                        description: 'Review the wording before anything reaches a customer.',
                      });
                    })
                  }
                  icon={<Sparkles className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
                >
                  Draft replies
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  isLoading={busy === 'approve'}
                  loadingText="Approving…"
                  disabled={pendingCount === 0}
                  onClick={() =>
                    void run('approve', async () => {
                      await apiClient.approveIncidentDrafts(incident.id);
                      toast.success('Replies approved', {
                        description: 'Approve moved them to the send queue.',
                      });
                    })
                  }
                  icon={<Check className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
                >
                  Approve all
                </Button>
                <Button
                  variant="primary"
                  size="sm"
                  isLoading={busy === 'send'}
                  loadingText="Sending…"
                  disabled={approvedCount === 0}
                  onClick={() =>
                    void run('send', async () => {
                      const result = await apiClient.sendIncidentDrafts(incident.id);
                      toast.success('Replies sent', {
                        description: `${result.sent.length} customer${result.sent.length === 1 ? '' : 's'} notified.`,
                      });
                    })
                  }
                  icon={<Send className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
                >
                  Send approved
                </Button>
              </div>
            </div>

            {drafts.length === 0 ? (
              <EmptyState
                icon={<FileText className="h-7 w-7" aria-hidden="true" strokeWidth={2.5} />}
                title="No replies drafted yet"
                description="Draft a reply for every customer in this incident, then approve the wording you are happy with."
              />
            ) : (
              <ul className="divide-y-2 divide-line">
                {drafts.map((draft) => (
                  <DraftRow
                    key={draft.id}
                    draft={draft}
                    busy={busy}
                    editing={editing === draft.id}
                    editBody={editBody}
                    setEditBody={setEditBody}
                    onEdit={() => {
                      setEditing(draft.id);
                      setEditBody(draft.body);
                    }}
                    onCancelEdit={() => setEditing(null)}
                    onSave={() =>
                      void run(`edit-${draft.id}`, async () => {
                        await apiClient.updateDraft(draft.id, editBody);
                        setEditing(null);
                        toast.success('Reply updated');
                      })
                    }
                    onApprove={() =>
                      void run(`approve-${draft.id}`, async () => {
                        await apiClient.approveDraft(draft.id);
                        toast.success('Reply approved');
                      })
                    }
                    onSend={() =>
                      void run(`send-${draft.id}`, async () => {
                        await apiClient.sendDraft(draft.id);
                        toast.success('Reply sent', {
                          description: 'The customer was notified and the send is on the audit timeline.',
                        });
                      })
                    }
                    onDiscard={() =>
                      void run(`discard-${draft.id}`, async () => {
                        await apiClient.discardDraft(draft.id);
                        toast.success('Reply discarded');
                      })
                    }
                  />
                ))}
              </ul>
            )}
          </section>

          <section className="brut-panel overflow-hidden rounded border-2 border-line bg-surface shadow-hard">
            <div className="border-b-2 border-line bg-surface-alt px-4 py-3">
              <h2 className="text-section font-bold text-ink">Linked cases</h2>
              <p className="mt-0.5 text-meta text-muted">
                Grouping can be corrected. Removing a case never changes that case’s own policy checks.
              </p>
            </div>
            <ul className="divide-y-2 divide-line">
              {incident.cases.map((item) => {
                const draft = draftsByCase.get(item.case_id);
                return (
                  <li key={item.case_id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                    <Link
                      to={`/merchant/cases/${item.case_id}`}
                      className="min-w-0 flex-1 focus-visible:outline focus-visible:outline-[3px] focus-visible:outline-offset-2 focus-visible:outline-[color:var(--brand-sky)]"
                    >
                      <span className="font-mono text-[13px] font-semibold text-ink">
                        {item.case_number}
                      </span>
                      <span className="ml-2 text-body text-ink">
                        {item.customer_name || item.customer_email}
                      </span>
                      <span className="ml-2 text-meta text-muted">{item.summary}</span>
                    </Link>
                    <StatusPill meta={statusMeta(item.status)} size="sm" withDot={false} />
                    <StatusPill meta={formatRisk(item.severity)} size="sm" />
                    <span className="font-mono text-[12px] text-muted">
                      {formatMinorUnits(item.amount_minor, item.currency)}
                    </span>
                    {draft ? (
                      <Tag tone={draftTone(draft.status)}>{draft.status.toLowerCase()}</Tag>
                    ) : null}
                    <Tooltip content="Remove this case from the incident" side="left">
                      <IconButton
                        label={`Remove ${item.case_number} from this incident`}
                        size="sm"
                        variant="outline"
                        disabled={busy === `remove-${item.case_id}`}
                        onClick={() =>
                          void run(`remove-${item.case_id}`, async () => {
                            await apiClient.removeIncidentCase(incident.id, item.case_id);
                            toast.success('Case removed from the incident');
                          })
                        }
                      >
                        <Link2Off className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
                      </IconButton>
                    </Tooltip>
                  </li>
                );
              })}
            </ul>
          </section>

          <p className="text-meta text-muted">
            First seen {formatAbsolute(incident.first_seen_at)} · last activity {formatAbsolute(incident.last_seen_at)}
          </p>
        </div>
      </div>
    </div>
  );
}

function DraftRow({
  draft,
  busy,
  editing,
  editBody,
  setEditBody,
  onEdit,
  onCancelEdit,
  onSave,
  onApprove,
  onSend,
  onDiscard,
}: {
  draft: MessageDraft;
  busy: string | null;
  editing: boolean;
  editBody: string;
  setEditBody: (value: string) => void;
  onEdit: () => void;
  onCancelEdit: () => void;
  onSave: () => void;
  onApprove: () => void;
  onSend: () => void;
  onDiscard: () => void;
}) {
  const isBusy = busy === `approve-${draft.id}` || busy === `send-${draft.id}` || busy === `discard-${draft.id}`;

  return (
    <li className="px-4 py-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-[13px] font-semibold text-ink">{draft.case_number}</span>
            <Tag tone={draftTone(draft.status)}>{draft.status.toLowerCase()}</Tag>
            <span className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">
              {formatDraftType(draft.draft_type)}
            </span>
            {draft.guardrail_flag ? (
              <span className="inline-flex items-center gap-1 rounded-pill border-2 border-line bg-warning px-1.5 py-0.5 text-[11px] font-bold uppercase tracking-[0.04em] text-[color:var(--warning-ink)]">
                <AlertTriangle className="h-3 w-3" aria-hidden="true" strokeWidth={3} />
                Merchant decision needed
              </span>
            ) : null}
          </div>
          <p className="mt-0.5 text-meta text-muted">
            {draft.customer_name || draft.customer_email}
            {draft.sent_at ? ` · sent ${formatTime(draft.sent_at)}` : ''}
          </p>
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {draft.status === 'DRAFTED' ? (
            <>
              <Button variant="outline" size="sm" onClick={onEdit} icon={<Pencil className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}>
                Edit
              </Button>
              <Button
                variant="secondary"
                size="sm"
                isLoading={busy === `approve-${draft.id}`}
                loadingText="Approving…"
                onClick={onApprove}
                icon={<Check className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
              >
                Approve
              </Button>
              <Button
                variant="outline"
                size="sm"
                isLoading={busy === `discard-${draft.id}`}
                loadingText="Discarding…"
                onClick={onDiscard}
                icon={<Trash2 className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
              >
                Discard
              </Button>
            </>
          ) : null}
          {draft.status === 'APPROVED' ? (
            <>
              <Button variant="outline" size="sm" onClick={onEdit} icon={<Pencil className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}>
                Edit
              </Button>
              <Button
                variant="primary"
                size="sm"
                isLoading={busy === `send-${draft.id}`}
                loadingText="Sending…"
                onClick={onSend}
                icon={<Send className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
              >
                Send
              </Button>
            </>
          ) : null}
          {draft.status === 'SENT' ? (
            <span className="inline-flex items-center gap-1 text-[11px] font-bold uppercase tracking-[0.04em] text-[color:var(--success-ink)]">
              <CheckCheck className="h-3.5 w-3.5" aria-hidden="true" strokeWidth={3} />
              Sent
            </span>
          ) : null}
        </div>
      </div>

      {editing ? (
        <div className="mt-3">
          <label htmlFor={`draft-${draft.id}`} className="meta-label">
            Reply wording
          </label>
          <textarea
            id={`draft-${draft.id}`}
            value={editBody}
            onChange={(event) => setEditBody(event.target.value)}
            rows={5}
            className={cn(
              'mt-1 w-full rounded border-2 border-line bg-surface p-2 text-body text-ink',
              'focus:outline focus:outline-[3px] focus:outline-offset-2 focus:outline-[color:var(--brand-sky)]',
            )}
          />
          <p className="mt-1 text-[11px] text-muted">
            Replies can state verified facts only. Promises about refunds, compensation or timelines are blocked.
          </p>
          <div className="mt-2 flex items-center gap-2">
            <Button variant="primary" size="sm" isLoading={isBusy} onClick={onSave}>
              Save wording
            </Button>
            <Button variant="ghost" size="sm" onClick={onCancelEdit}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <p className="mt-2 whitespace-pre-wrap border-2 border-line bg-surface-alt p-3 text-body text-ink">
          {draft.body}
        </p>
      )}
    </li>
  );
}

export default MerchantIncidentDetail;
