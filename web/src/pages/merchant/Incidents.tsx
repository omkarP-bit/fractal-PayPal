import { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  Inbox,
  Layers,
  RefreshCw,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import { toast } from 'sonner';
import { apiClient, type Incident, type IncidentCase, type TriageInbox } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import {
  apiErrorMessage,
  formatAbsolute,
  formatMinorUnits,
  formatRisk,
  formatTime,
  statusMeta,
} from '../../lib/format';
import { PageHeader } from '../../components/ui/Panel';
import { Button } from '../../components/ui/Button';
import { Select } from '../../components/ui/Input';
import { StatusPill, Tag } from '../../components/ui/StatusPill';
import { StatBlock } from '../../components/ui/StatBlock';
import { Tooltip } from '../../components/ui/Tooltip';
import { EmptyState, ErrorNotice, Skeleton } from '../../components/ui/States';

type ScopeFilter = 'all' | 'incidents' | 'cases';

const SCOPE_OPTIONS: Array<{ value: ScopeFilter; label: string }> = [
  { value: 'all', label: 'Everything' },
  { value: 'incidents', label: 'Incidents only' },
  { value: 'cases', label: 'Cases only' },
];

const SORT_OPTIONS: Array<{ value: 'priority' | 'amount' | 'age'; label: string }> = [
  { value: 'priority', label: 'Priority first' },
  { value: 'amount', label: 'Largest amount first' },
  { value: 'age', label: 'Oldest first' },
];

function severityTone(severity: string): 'neutral' | 'warning' | 'danger' | 'success' | 'info' {
  switch (severity.toUpperCase()) {
    case 'CRITICAL':
      return 'danger';
    case 'HIGH':
      return 'warning';
    case 'MEDIUM':
      return 'info';
    default:
      return 'neutral';
  }
}

export function MerchantIncidents() {
  const [data, setData] = useState<TriageInbox | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [scope, setScope] = useState<ScopeFilter>('all');
  const [sort, setSort] = useState<'priority' | 'amount' | 'age'>('priority');
  const [regrouping, setRegrouping] = useState(false);
  const { logout } = useAuthStore();
  const navigate = useNavigate();

  const fetchInbox = useCallback(async () => {
    setLoading(true);
    try {
      const result = await apiClient.getTriageInbox();
      setData(result);
      setError(null);
    } catch (err) {
      const raw = err instanceof Error ? err.message : '';
      setError(apiErrorMessage(err, 'We couldn’t reach the case assistant. Try again.'));
      if (raw.includes('401')) {
        logout();
        navigate('/login');
      }
    } finally {
      setLoading(false);
    }
  }, [logout, navigate]);

  useEffect(() => {
    void fetchInbox();
  }, [fetchInbox]);

  const regroup = async () => {
    setRegrouping(true);
    try {
      await apiClient.regroupCases();
      await fetchInbox();
      toast.success('Cases regrouped', {
        description: 'Incidents now reflect the latest verified facts.',
      });
    } catch (err) {
      toast.error(apiErrorMessage(err, 'We couldn’t regroup the cases.'));
    } finally {
      setRegrouping(false);
    }
  };

  const incidents = data?.incidents ?? [];
  const standalone = useMemo(() => {
    const grouped = new Set<string>();
    incidents.forEach((incident) => incident.cases.forEach((c) => grouped.add(c.case_id)));
    return (data?.cases ?? []).filter((c) => !grouped.has(c.case_id));
  }, [data, incidents]);

  const sortedIncidents = useMemo(() => {
    const items = [...incidents];
    if (sort === 'amount') {
      items.sort((a, b) => b.affected_amount_minor - a.affected_amount_minor);
    } else if (sort === 'age') {
      items.sort((a, b) => a.first_seen_at.localeCompare(b.first_seen_at));
    } else {
      items.sort((a, b) => b.priority_score - a.priority_score);
    }
    return items;
  }, [incidents, sort]);

  const sortedCases = useMemo(() => {
    const items = [...standalone];
    if (sort === 'amount') {
      items.sort((a, b) => b.amount_minor - a.amount_minor);
    } else if (sort === 'age') {
      items.sort((a, b) => a.created_at.localeCompare(b.created_at));
    } else {
      items.sort((a, b) => b.priority_score - a.priority_score);
    }
    return items;
  }, [standalone, sort]);

  const totals = useMemo(
    () => ({
      incidents: incidents.length,
      cases: incidents.reduce((sum, incident) => sum + incident.case_count, 0),
      exposure: incidents.reduce((sum, incident) => sum + incident.affected_amount_minor, 0),
      drafts: incidents.reduce((sum, incident) => sum + incident.drafts_pending, 0),
    }),
    [incidents],
  );

  const showIncidents = scope !== 'cases';
  const showCases = scope !== 'incidents';

  if (loading) {
    return (
      <div className="page-fill gap-6">
        <PageHeader title="Incident inbox" description="Grouped exceptions, ranked by what needs attention." />
        <div className="grid gap-4 lg:grid-cols-2">
          {[0, 1, 2, 3].map((key) => (
            <div key={key} className="rounded border-2 border-line bg-surface p-5">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="mt-3 h-5 w-3/4" />
              <Skeleton className="mt-2 h-3 w-1/2" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page-fill gap-6">
        <PageHeader title="Incident inbox" description="Grouped exceptions, ranked by what needs attention." />
        <ErrorNotice
          title="We couldn’t load the inbox"
          description={error}
          onRetry={() => void fetchInbox()}
        />
      </div>
    );
  }

  const empty = incidents.length === 0 && standalone.length === 0;

  return (
    <div className="page-fill">
      <div className="list-layout flex-1">
        <div className="flex min-w-0 flex-col gap-5">
          <PageHeader
            title="Incident inbox"
            description="Grouped exceptions, ranked by what needs attention."
            actions={
              <Button
                variant="outline"
                size="sm"
                isLoading={regrouping}
                loadingText="Regrouping…"
                onClick={() => void regroup()}
                icon={<RefreshCw className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
              >
                Regroup
              </Button>
            }
          />

          <div className="stat-strip">
            <div className="stat-strip-grid">
              <StatBlock label="Open incidents" value={totals.incidents} tone="info" />
              <StatBlock label="Cases affected" value={totals.cases} tone="warning" />
              <StatBlock label="Exposure" value={Math.round(totals.exposure / 100)} tone="danger" />
              <StatBlock label="Drafts to review" value={totals.drafts} tone="neutral" />
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Select
              aria-label="Filter the inbox"
              value={scope}
              onChange={(event) => setScope(event.target.value as ScopeFilter)}
              options={SCOPE_OPTIONS}
              className="h-11 w-auto min-w-[150px] text-meta"
            />
            <Select
              aria-label="Sort the inbox"
              value={sort}
              onChange={(event) => setSort(event.target.value as 'priority' | 'amount' | 'age')}
              options={SORT_OPTIONS}
              className="h-11 w-auto min-w-[180px] text-meta"
            />
          </div>

          {empty ? (
            <div className="brut-panel rounded border-2 border-line bg-surface shadow-hard">
              <EmptyState
                icon={<Inbox className="h-7 w-7" aria-hidden="true" strokeWidth={2.5} />}
                title="Nothing needs triage right now"
                description="When customers report the same failure, the assistant groups the cases into one incident and ranks it here."
                action={
                  <Button variant="outline" size="sm" onClick={() => void regroup()}>
                    Check for new groups
                  </Button>
                }
              />
            </div>
          ) : null}

          {showIncidents && sortedIncidents.length > 0 && (
            <section aria-labelledby="incidents-heading" className="flex flex-col gap-3">
              <h2 id="incidents-heading" className="text-section font-bold text-ink">
                Incidents
              </h2>
              <div className="grid gap-4 lg:grid-cols-2">
                {sortedIncidents.map((incident) => (
                  <IncidentCard key={incident.id} incident={incident} />
                ))}
              </div>
            </section>
          )}

          {showCases && sortedCases.length > 0 && (
            <section aria-labelledby="standalone-heading" className="flex flex-col gap-3">
              <h2 id="standalone-heading" className="text-section font-bold text-ink">
                Single cases
              </h2>
              <div className="brut-panel overflow-hidden rounded border-2 border-line bg-surface shadow-hard">
                <ul className="divide-y-2 divide-line">
                  {sortedCases.map((item) => (
                    <StandaloneCaseRow key={item.case_id} item={item} />
                  ))}
                </ul>
              </div>
            </section>
          )}
        </div>
      </div>
    </div>
  );
}

function IncidentCard({ incident }: { incident: Incident }) {
  return (
    <article className="lift flex flex-col gap-3 rounded border-2 border-line bg-surface p-4 shadow-hard">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-[13px] font-bold text-ink">{incident.number}</span>
            <StatusPill meta={formatRisk(incident.severity)} size="sm" />
            <StatusPill meta={statusMeta(incident.status)} size="sm" withDot={false} />
          </div>
          <h3 className="mt-2 text-body font-bold leading-snug text-ink">
            {incident.root_cause_summary}
          </h3>
        </div>
        <Tooltip content={`Priority ${incident.priority_score}`} side="left">
          <span className="shrink-0 rounded border-2 border-line bg-info px-2 py-1 font-mono text-[12px] font-bold text-[color:var(--info-ink)]">
            P{incident.priority_score}
          </span>
        </Tooltip>
      </div>

      <p className="flex items-start gap-2 text-meta text-muted">
        <Sparkles className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" strokeWidth={2.5} />
        <span>{incident.ai_summary}</span>
      </p>

      <dl className="grid grid-cols-3 gap-2 text-center">
        <div className="rounded border-2 border-line bg-surface-alt px-2 py-1.5">
          <dt className="meta-label">Open cases</dt>
          <dd className="text-body font-bold text-ink">
            {incident.open_case_count}
            <span className="text-[11px] font-normal text-muted"> / {incident.case_count}</span>
          </dd>
        </div>
        <div className="rounded border-2 border-line bg-surface-alt px-2 py-1.5">
          <dt className="meta-label">Amount</dt>
          <dd className="text-body font-bold text-ink">
            {formatMinorUnits(incident.affected_amount_minor, incident.currency)}
          </dd>
        </div>
        <div className="rounded border-2 border-line bg-surface-alt px-2 py-1.5">
          <dt className="meta-label">Confidence</dt>
          <dd className="text-body font-bold text-ink">
            {Math.round(incident.confidence * 100)}%
          </dd>
        </div>
      </dl>

      <p className="flex items-start gap-2 text-meta text-muted">
        <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0" aria-hidden="true" strokeWidth={2.5} />
        <span>{incident.next_step}</span>
      </p>

      <div className="flex flex-wrap items-center gap-2 border-t-2 border-line pt-3">
        <Link
          to={`/merchant/incidents/${incident.id}`}
          className="btn btn-lift inline-flex h-9 items-center justify-center gap-2 rounded bg-yellow px-3 text-meta font-extrabold text-[color:var(--text-on-yellow)] hover:bg-yellow-hover"
        >
          <Layers className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
          Open incident
        </Link>
        {incident.drafts_pending > 0 && (
          <Tag tone="warning">{incident.drafts_pending} drafts to review</Tag>
        )}
        {incident.drafts_sent > 0 && (
          <Tag tone="success">{incident.drafts_sent} replies sent</Tag>
        )}
      </div>
    </article>
  );
}

function StandaloneCaseRow({ item }: { item: IncidentCase }) {
  return (
    <li className="flex flex-wrap items-center gap-3 px-4 py-3">
      <Link
        to={`/merchant/cases/${item.case_id}`}
        className="min-w-0 flex-1 focus-visible:outline focus-visible:outline-[3px] focus-visible:outline-offset-2 focus-visible:outline-[color:var(--brand-sky)]"
      >
        <span className="font-mono text-[13px] font-semibold text-ink">{item.case_number}</span>
        <span className="ml-2 text-body text-ink">
          {item.customer_name || item.customer_email}
        </span>
        <span className="ml-2 text-meta text-muted">{item.summary}</span>
      </Link>
      <StatusPill meta={formatRisk(item.severity)} size="sm" />
      <span className="font-mono text-[12px] text-muted">
        {formatMinorUnits(item.amount_minor, item.currency)}
      </span>
      <span className="font-mono text-[12px] text-muted">
        <Tooltip content={formatAbsolute(item.created_at)} side="left">
          <time dateTime={item.created_at}>{formatTime(item.created_at)}</time>
        </Tooltip>
      </span>
    </li>
  );
}

export default MerchantIncidents;
