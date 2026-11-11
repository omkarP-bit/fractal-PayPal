import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowUpRight, Inbox, Plus, SearchX } from 'lucide-react';
import { apiClient, type CaseSummary } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { useGlobalSearch } from '../../lib/search';
import {
  apiErrorMessage,
  formatAbsolute,
  formatMinorUnits,
  formatTime,
  statusMeta,
} from '../../lib/format';
import { cn } from '../../lib/cn';
import { PageHeader } from '../../components/ui/Panel';
import { Button } from '../../components/ui/Button';
import { StatusPill } from '../../components/ui/StatusPill';
import { StatBlock } from '../../components/ui/StatBlock';
import { CopyChip } from '../../components/ui/CopyChip';
import { Tooltip } from '../../components/ui/Tooltip';
import { Highlight } from '../../components/ui/Highlight';
import { EmptyState, ErrorNotice, Skeleton } from '../../components/ui/States';
import { InsightRail, type RailItem } from '../../components/InsightRail';

type CustomerFilter = 'all' | 'open' | 'resolved' | 'awaiting';

const RESOLVED_STATUSES = ['RESOLVED', 'CLOSED', 'REJECTED'];
const AWAITING_STATUSES = ['OPEN', 'COLLECTING_EVIDENCE'];

function matchesFilter(status: string, filter: CustomerFilter): boolean {
  switch (filter) {
    case 'resolved':
      return RESOLVED_STATUSES.includes(status);
    case 'awaiting':
      return AWAITING_STATUSES.includes(status);
    case 'open':
      return !RESOLVED_STATUSES.includes(status);
    default:
      return true;
  }
}

export function CustomerDashboard() {
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [filter, setFilter] = useState<CustomerFilter>('all');
  const [visibleCount, setVisibleCount] = useState(12);
  const { query, clear } = useGlobalSearch();
  const { logout } = useAuthStore();
  const navigate = useNavigate();

  const fetchCases = async () => {
    setLoading(true);
    try {
      const data = await apiClient.getCases();
      setCases(data);
      setError(null);
    } catch (err) {
      const raw = err instanceof Error ? err.message : '';
      setError(apiErrorMessage(err, 'We couldn’t reach the case service. Try again.'));
      if (raw.includes('401')) {
        logout();
        navigate('/login');
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void fetchCases();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(query.trim()), 150);
    return () => window.clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    setVisibleCount(12);
  }, [debouncedQuery, filter]);

  const counts = useMemo(
    () => ({
      all: cases.length,
      open: cases.filter((item) => matchesFilter(item.status, 'open')).length,
      resolved: cases.filter((item) => matchesFilter(item.status, 'resolved')).length,
      awaiting: cases.filter((item) => matchesFilter(item.status, 'awaiting')).length,
    }),
    [cases],
  );

  const filteredCases = useMemo(() => {
    const needle = debouncedQuery.toLowerCase();
    return cases
      .filter((item) => {
        if (!matchesFilter(item.status, filter)) return false;
        if (!needle) return true;
        return [item.case_number, item.merchant?.name, item.order?.ref, item.customer?.email]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
          .includes(needle);
      })
      .sort(
        (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
      );
  }, [cases, debouncedQuery, filter]);

  const visibleCases = filteredCases.slice(0, visibleCount);

  const attention: RailItem[] = filteredCases
    .filter((item) => matchesFilter(item.status, 'awaiting'))
    .slice(0, 5)
    .map((item) => ({
      id: item.id,
      caseNumber: item.case_number,
      status: item.status,
      meta: [item.merchant?.name, item.order?.ref].filter(Boolean).join(' · '),
      updatedAt: item.updated_at,
    }));

  const activity: RailItem[] = [...cases]
    .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    .slice(0, 5)
    .map((item) => ({
      id: item.id,
      caseNumber: item.case_number,
      status: item.status,
      meta: [statusMeta(item.status).label, item.merchant?.name].filter(Boolean).join(' · '),
      updatedAt: item.updated_at,
    }));

  const summary = [
    { key: 'open' as const, label: 'Open', value: counts.open, tone: 'info' as const },
    { key: 'resolved' as const, label: 'Resolved', value: counts.resolved, tone: 'success' as const },
    { key: 'awaiting' as const, label: 'Awaiting you', value: counts.awaiting, tone: 'warning' as const },
    { key: 'all' as const, label: 'Total', value: counts.all, tone: 'neutral' as const },
  ];

  if (loading) {
    return (
      <div className="page-fill gap-5">
        <PageHeader title="My cases" description="Track payment issues and replies from your merchant." />
        <Skeleton className="h-[92px] w-full" />
        <div className="rounded border-2 border-line bg-surface shadow-hard">
          {Array.from({ length: 5 }).map((_, index) => (
            <div
              key={index}
              className="flex items-center gap-6 border-b-2 border-line px-4 py-4 last:border-0"
            >
              <Skeleton className="h-4 w-16" />
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-4 w-36" />
              <Skeleton className="ml-auto h-6 w-24" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page-fill gap-5">
        <PageHeader title="My cases" description="Track payment issues and replies from your merchant." />
        <ErrorNotice
          title="We couldn’t load your cases"
          description={error}
          onRetry={() => void fetchCases()}
        />
      </div>
    );
  }

  return (
    <div className="page-fill">
      <div className="list-layout flex-1">
        <div className="flex min-w-0 flex-col gap-5">
          <PageHeader
            title="My cases"
            description="Track payment issues and replies from your merchant."
            actions={
              <Button
                variant="primary"
                onClick={() => navigate('/customer/cases/new')}
                icon={<Plus className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />}
              >
                Report an issue
              </Button>
            }
          />

          {debouncedQuery && (
            <p className="text-meta font-bold text-muted" aria-live="polite">
              {filteredCases.length} {filteredCases.length === 1 ? 'result' : 'results'} for “{debouncedQuery}”
            </p>
          )}

          <div className="stat-strip">
            <div className="stat-strip-grid">
              {summary.map((item) => (
                <StatBlock
                  key={item.key}
                  label={item.label}
                  value={item.value}
                  tone={item.tone === 'info' || item.tone === 'success' ? 'neutral' : item.tone}
                  active={filter === item.key && item.key !== 'all'}
                  onClick={() => setFilter(filter === item.key && item.key !== 'all' ? 'all' : item.key)}
                />
              ))}
            </div>
          </div>

          <div className="brut-panel overflow-hidden rounded border-2 border-line bg-surface shadow-hard">
            <div className="hidden xl:block">
              <table className="data-table">
                <caption className="sr-only">
                  Your payment exception cases, with merchant, payment, order and status.
                </caption>
                <colgroup>
                  <col style={{ width: 120 }} />
                  <col style={{ width: '20%' }} />
                  <col style={{ width: '17%' }} />
                  <col style={{ width: '27%' }} />
                  <col style={{ width: 150 }} />
                  <col style={{ width: 120 }} />
                </colgroup>
                <thead>
                  <tr>
                    <th scope="col">Case</th>
                    <th scope="col">Merchant</th>
                    <th scope="col">Payment</th>
                    <th scope="col">Order</th>
                    <th scope="col">Status</th>
                    <th scope="col" className="text-right">
                      Updated
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {visibleCases.map((item) => {
                    const href = `/customer/cases/${item.id}`;
                    const paymentLabel = item.payment ? statusMeta(item.payment.status).label : null;
                    return (
                      <tr key={item.id} className="group/row relative">
                        <td>
                          <Link
                            to={href}
                            aria-label={`Open case ${item.case_number}`}
                            className="table-row-link focus-visible:outline focus-visible:outline-[3px] focus-visible:outline-offset-[-3px] focus-visible:outline-[color:var(--brand-sky)]"
                          />
                          <span className="font-mono text-[13px] font-semibold text-ink">
                            <Highlight text={item.case_number} query={debouncedQuery} />
                          </span>
                        </td>
                        <td>
                          <p className="truncate text-body font-bold text-ink">
                            <Highlight text={item.merchant?.name || 'Merchant'} query={debouncedQuery} />
                          </p>
                        </td>
                        <td>
                          <p className="text-body font-bold tabular-nums text-ink">
                            {item.payment
                              ? formatMinorUnits(item.payment.amount_minor, item.payment.currency)
                              : '—'}
                          </p>
                          {paymentLabel && (
                            <p className="text-[12px] text-muted">{paymentLabel}</p>
                          )}
                        </td>
                        <td>
                          <div className="flex min-w-0 items-center gap-1">
                            <span className="min-w-0 truncate font-mono text-[12px] font-medium text-ink">
                              <Highlight text={item.order?.ref ?? '—'} query={debouncedQuery} />
                            </span>
                            {item.order?.ref && (
                              <span className="relative z-20 shrink-0">
                                <CopyChip
                                  value={item.order.ref}
                                  title={`Copy order ${item.order.ref}`}
                                  iconOnly
                                  reveal
                                  className="[&>button]:h-6 [&>button]:border-0 [&>button]:bg-transparent [&>button]:px-1"
                                />
                              </span>
                            )}
                          </div>
                          {item.order && (
                            <div className="mt-1">
                              <StatusPill meta={statusMeta(item.order.status)} size="sm" withDot={false} />
                            </div>
                          )}
                        </td>
                        <td>
                          <StatusPill meta={statusMeta(item.status)} />
                        </td>
                        <td className="text-right">
                          <Tooltip content={formatAbsolute(item.updated_at)} side="left">
                            <time
                              dateTime={item.updated_at}
                              className="font-mono text-[12px] text-muted"
                            >
                              {formatTime(item.updated_at)}
                            </time>
                          </Tooltip>
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            <div className="grid gap-4 p-4 sm:grid-cols-2 xl:hidden">
              {visibleCases.map((item) => (
                <Link
                  key={item.id}
                  to={`/customer/cases/${item.id}`}
                  className="lift block rounded border-2 border-line bg-surface p-4 shadow-hard"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="font-mono text-[13px] font-semibold text-ink">{item.case_number}</p>
                      <p className="mt-0.5 truncate text-meta text-muted">
                        {item.merchant?.name || 'Merchant'}
                      </p>
                    </div>
                    <StatusPill meta={statusMeta(item.status)} size="sm" />
                  </div>
                  <div className="mt-3 flex items-center justify-between gap-3 text-meta text-ink">
                    <span className="font-bold tabular-nums">
                      {item.payment
                        ? formatMinorUnits(item.payment.amount_minor, item.payment.currency)
                        : '—'}
                    </span>
                    <span className="truncate font-mono text-[12px]">{item.order?.ref ?? '—'}</span>
                  </div>
                  <p className="mt-1.5 font-mono text-[11px] text-muted">
                    {formatTime(item.updated_at)}
                  </p>
                </Link>
              ))}
            </div>

            {filteredCases.length === 0 && (
              <EmptyState
                icon={
                  cases.length === 0 ? (
                    <Inbox className="h-7 w-7" strokeWidth={2} />
                  ) : (
                    <SearchX className="h-7 w-7" strokeWidth={2} />
                  )
                }
                title={cases.length === 0 ? 'No cases yet' : 'No cases match this view'}
                description={
                  cases.length === 0
                    ? 'When a payment doesn’t match your order, report it here and we’ll help you sort it out.'
                    : 'Try a different filter, case number, merchant or order reference.'
                }
                action={
                  cases.length === 0 ? (
                    <Button
                      variant="primary"
                      onClick={() => navigate('/customer/cases/new')}
                      icon={<Plus className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />}
                    >
                      Report an issue
                    </Button>
                  ) : (
                    <Button
                      variant="outline"
                      onClick={() => {
                        setFilter('all');
                        clear();
                      }}
                    >
                      Clear filters
                    </Button>
                  )
                }
              />
            )}

            {filteredCases.length > 0 && (
              <div className="flex flex-col gap-3 border-t-2 border-line px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <p className="font-mono text-[12px] text-muted">
                  Showing {visibleCases.length} of {filteredCases.length}{' '}
                  {filteredCases.length === 1 ? 'case' : 'cases'}
                </p>
                <p className="hidden text-meta text-muted lg:block">
                  Press <kbd className="rounded-pill border border-line bg-surface-alt px-1 font-mono text-[11px] font-bold text-ink">/</kbd> to search
                </p>
                <div className="flex items-center gap-2">
                  {visibleCount < filteredCases.length ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setVisibleCount((count) => count + 12)}
                    >
                      Load more
                    </Button>
                  ) : (
                    <span className="font-mono text-[12px] text-muted">
                      Page 1 of 1
                    </span>
                  )}
                </div>
              </div>
            )}
          </div>

          {cases.length > 0 && (
            <div className="mt-auto flex items-center gap-2 rounded border-2 border-dashed border-line px-4 py-3 text-meta text-muted">
              <ArrowUpRight className="h-4 w-4 shrink-0" aria-hidden="true" strokeWidth={2.5} />
              Open a case to message your merchant or add evidence.
            </div>
          )}
        </div>

        <div className="list-rail">
          <InsightRail
            attention={attention}
            activity={activity}
            basePath="/customer/cases"
            emptyAttention="Nothing needs your attention. We’ll message you when there’s an update."
          />
        </div>
      </div>
    </div>
  );
}
