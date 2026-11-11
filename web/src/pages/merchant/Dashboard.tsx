import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowUpDown, Inbox, ListFilter, SearchX, X } from 'lucide-react';
import { apiClient, type CaseSummary } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { useGlobalSearch } from '../../lib/search';
import { cn } from '../../lib/cn';
import {
  apiErrorMessage,
  formatAbsolute,
  formatCaseType,
  formatTime,
  statusMeta,
} from '../../lib/format';
import { PageHeader } from '../../components/ui/Panel';
import { Button, IconButton } from '../../components/ui/Button';
import { Select } from '../../components/ui/Input';
import { StatusPill } from '../../components/ui/StatusPill';
import { StatBlock } from '../../components/ui/StatBlock';
import { CopyChip } from '../../components/ui/CopyChip';
import { Tooltip } from '../../components/ui/Tooltip';
import { Highlight } from '../../components/ui/Highlight';
import { EmptyState, ErrorNotice, Skeleton } from '../../components/ui/States';
import { InsightRail, type RailItem } from '../../components/InsightRail';

type StatusFilter = 'all' | 'open' | 'investigating' | 'needs-approval' | 'escalated' | 'resolved';
type SortOption = 'attention' | 'newest' | 'oldest' | 'updated';

const STATUS_OPTIONS: Array<{ value: StatusFilter; label: string }> = [
  { value: 'all', label: 'All statuses' },
  { value: 'needs-approval', label: 'Needs approval' },
  { value: 'investigating', label: 'Investigating' },
  { value: 'escalated', label: 'Escalated' },
  { value: 'resolved', label: 'Resolved' },
  { value: 'open', label: 'Open' },
];

const SORT_OPTIONS: Array<{ value: SortOption; label: string }> = [
  { value: 'attention', label: 'Needs attention first' },
  { value: 'newest', label: 'Newest first' },
  { value: 'oldest', label: 'Oldest first' },
  { value: 'updated', label: 'Recently updated' },
];

function matchesStatus(status: string, filter: StatusFilter): boolean {
  switch (filter) {
    case 'needs-approval':
      return status === 'AWAITING_MERCHANT_APPROVAL';
    case 'investigating':
      return status === 'INVESTIGATING' || status === 'COLLECTING_EVIDENCE';
    case 'escalated':
      return status === 'ESCALATED';
    case 'resolved':
      return status === 'RESOLVED' || status === 'CLOSED';
    case 'open':
      return status === 'OPEN';
    default:
      return true;
  }
}

function needsAction(status: string): boolean {
  return status === 'AWAITING_MERCHANT_APPROVAL' || status === 'ESCALATED';
}

export function MerchantDashboard() {
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [typeFilter, setTypeFilter] = useState('all');
  const [sort, setSort] = useState<SortOption>('attention');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [visibleCount, setVisibleCount] = useState(12);
  const { query, clear: clearSearch } = useGlobalSearch();
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
  }, [debouncedQuery, statusFilter, typeFilter, sort]);

  const counts = useMemo(
    () => ({
      all: cases.length,
      needs: cases.filter((item) => item.status === 'AWAITING_MERCHANT_APPROVAL').length,
      investigating: cases.filter(
        (item) => item.status === 'INVESTIGATING' || item.status === 'COLLECTING_EVIDENCE',
      ).length,
      escalated: cases.filter((item) => item.status === 'ESCALATED').length,
    }),
    [cases],
  );

  const typeOptions = useMemo(() => {
    const seen = new Map<string, string>();
    cases.forEach((item) => {
      if (item.case_type && !seen.has(item.case_type)) {
        seen.set(item.case_type, formatCaseType(item.case_type));
      }
    });
    return [
      { value: 'all', label: 'All issue types' },
      ...Array.from(seen.entries())
        .sort((a, b) => a[1].localeCompare(b[1]))
        .map(([value, label]) => ({ value, label })),
    ];
  }, [cases]);

  const filteredCases = useMemo(() => {
    const needle = debouncedQuery.toLowerCase();

    const matches = cases.filter((item) => {
      if (!matchesStatus(item.status, statusFilter)) return false;
      if (typeFilter !== 'all' && item.case_type !== typeFilter) return false;
      if (!needle) return true;

      const haystack = [
        item.case_number,
        item.customer?.name,
        item.customer?.email,
        item.order?.ref,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();
      return haystack.includes(needle);
    });

    const byNewest = (a: CaseSummary, b: CaseSummary) =>
      new Date(b.created_at).getTime() - new Date(a.created_at).getTime();
    const byUpdated = (a: CaseSummary, b: CaseSummary) =>
      new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();

    return matches.sort((a, b) => {
      switch (sort) {
        case 'newest':
          return byNewest(a, b);
        case 'oldest':
          return -byNewest(a, b);
        case 'updated':
          return byUpdated(a, b);
        default:
          if (needsAction(a.status) !== needsAction(b.status)) {
            return needsAction(a.status) ? -1 : 1;
          }
          if (a.status === 'ESCALATED' && b.status === 'AWAITING_MERCHANT_APPROVAL') return -1;
          if (b.status === 'ESCALATED' && a.status === 'AWAITING_MERCHANT_APPROVAL') return 1;
          return byUpdated(a, b);
      }
    });
  }, [cases, statusFilter, typeFilter, debouncedQuery, sort]);

  const visibleCases = filteredCases.slice(0, visibleCount);

  const activeFilters: Array<{ key: string; label: string; remove: () => void }> = [];
  if (statusFilter !== 'all') {
    activeFilters.push({
      key: 'status',
      label: STATUS_OPTIONS.find((option) => option.value === statusFilter)?.label ?? 'Status',
      remove: () => setStatusFilter('all'),
    });
  }
  if (typeFilter !== 'all') {
    activeFilters.push({
      key: 'type',
      label: formatCaseType(typeFilter),
      remove: () => setTypeFilter('all'),
    });
  }
  if (query.trim()) {
    activeFilters.push({
      key: 'search',
      label: `Search: ${query.trim()}`,
      remove: clearSearch,
    });
  }

  const clearAll = () => {
    setStatusFilter('all');
    setTypeFilter('all');
    clearSearch();
  };

  const summary = [
    { key: 'all' as const, label: 'Total cases', value: counts.all, tone: 'neutral' as const },
    { key: 'investigating' as const, label: 'Investigating', value: counts.investigating, tone: 'neutral' as const },
    { key: 'needs-approval' as const, label: 'Needs approval', value: counts.needs, tone: 'warning' as const },
    { key: 'escalated' as const, label: 'Escalated', value: counts.escalated, tone: 'danger' as const },
  ];

  const attention: RailItem[] = [...cases]
    .filter((item) => needsAction(item.status))
    .sort((a, b) => {
      if (a.status !== b.status) return a.status === 'ESCALATED' ? -1 : 1;
      return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
    })
    .slice(0, 5)
    .map((item) => ({
      id: item.id,
      caseNumber: item.case_number,
      status: item.status,
      meta: [item.customer?.name || item.customer?.email, item.order?.ref]
        .filter(Boolean)
        .join(' · '),
      updatedAt: item.updated_at,
    }));

  const activity: RailItem[] = [...cases]
    .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
    .slice(0, 5)
    .map((item) => ({
      id: item.id,
      caseNumber: item.case_number,
      status: item.status,
      meta: [statusMeta(item.status).label, item.customer?.name || item.customer?.email]
        .filter(Boolean)
        .join(' · '),
      updatedAt: item.updated_at,
    }));

  if (loading) {
    return (
      <div className="page-fill gap-6">
        <PageHeader title="Cases" description="Payment exceptions for your store." />
        <Skeleton className="h-[92px] w-full" />
        <div className="rounded border-2 border-line bg-surface shadow-hard">
          {Array.from({ length: 5 }).map((_, index) => (
            <div
              key={index}
              className="flex items-center gap-6 border-b-2 border-line px-4 py-4 last:border-0"
            >
              <Skeleton className="h-4 w-16" />
              <Skeleton className="h-4 w-40" />
              <Skeleton className="h-4 w-28" />
              <Skeleton className="h-4 w-40" />
              <Skeleton className="ml-auto h-6 w-24" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="page-fill gap-6">
        <PageHeader title="Cases" description="Payment exceptions for your store." />
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
          <PageHeader title="Cases" description="Payment exceptions for your store." />

          <div className="stat-strip">
            <div className="stat-strip-grid">
              {summary.map((item) => (
                <StatBlock
                  key={item.key}
                  label={item.label}
                  value={item.value}
                  tone={item.tone}
                  active={statusFilter === item.key && item.key !== 'all'}
                  onClick={() =>
                    setStatusFilter(
                      statusFilter === item.key && item.key !== 'all' ? 'all' : item.key,
                    )
                  }
                />
              ))}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Select
              aria-label="Filter by status"
              value={statusFilter}
              onChange={(event) => setStatusFilter(event.target.value as StatusFilter)}
              options={STATUS_OPTIONS}
              className="h-11 w-auto min-w-[150px] text-meta"
            />
            <Select
              aria-label="Filter by issue type"
              value={typeFilter}
              onChange={(event) => setTypeFilter(event.target.value)}
              options={typeOptions}
              className="h-11 w-auto min-w-[150px] text-meta"
            />
            <Select
              aria-label="Sort cases"
              value={sort}
              onChange={(event) => setSort(event.target.value as SortOption)}
              options={SORT_OPTIONS}
              className="h-11 w-auto min-w-[180px] text-meta"
            />
          </div>

          {activeFilters.length > 0 && (
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center gap-1 text-[12px] font-bold text-muted">
                <ListFilter className="h-3.5 w-3.5" aria-hidden="true" strokeWidth={2.5} />
                Filtered
              </span>
              {activeFilters.map((filter) => (
                <span
                  key={filter.key}
                  className="inline-flex max-w-full items-center gap-1 rounded-pill border-2 border-line bg-info py-0.5 pl-2 pr-0.5 text-[12px] font-bold text-[color:var(--info-ink)]"
                >
                  <span className="truncate">{filter.label}</span>
                  <IconButton
                    label={`Remove ${filter.key} filter`}
                    size="sm"
                    onClick={filter.remove}
                    className="h-8 w-8 border-0 bg-transparent shadow-none hover:bg-surface"
                  >
                    <X className="h-3 w-3" aria-hidden="true" strokeWidth={3} />
                  </IconButton>
                </span>
              ))}
              <Button variant="tertiary" size="sm" onClick={clearAll} className="h-8 px-1.5">
                Clear all
              </Button>
            </div>
          )}

          <div className="brut-panel overflow-hidden rounded border-2 border-line bg-surface shadow-hard">
            <div className="hidden xl:block">
              <table className="data-table">
                <caption className="sr-only">
                  Open cases for your store, with customer, order, issue, status and last update.
                </caption>
                <colgroup>
                  <col style={{ width: 120 }} />
                  <col style={{ width: '22%' }} />
                  <col style={{ width: '24%' }} />
                  <col style={{ width: '18%' }} />
                  <col style={{ width: 150 }} />
                  <col style={{ width: 120 }} />
                </colgroup>
                <thead>
                  <tr>
                    <th scope="col">Case</th>
                    <th scope="col">Customer</th>
                    <th scope="col">Order</th>
                    <th scope="col">Issue</th>
                    <th scope="col">Status</th>
                    <th scope="col" className="text-right">
                      Updated
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {visibleCases.map((item) => {
                    const action = needsAction(item.status);
                    const href = `/merchant/cases/${item.id}`;
                    return (
                      <tr key={item.id} className="group/row relative">
                        <td>
                          {action && (
                            <span
                              className={cn(
                                'absolute inset-y-0 left-0 z-10 w-1.5',
                                item.status === 'ESCALATED' ? 'bg-danger' : 'bg-warning',
                              )}
                              aria-hidden="true"
                            />
                          )}
                          <Link
                            to={href}
                            aria-label={`Open case ${item.case_number}`}
                            className="table-row-link focus-visible:outline focus-visible:outline-[3px] focus-visible:outline-offset-[-3px] focus-visible:outline-[color:var(--brand-sky)]"
                          />
                          <span className="font-mono text-[13px] font-semibold text-ink">
                            {item.case_number}
                          </span>
                        </td>
                        <td>
                          <p className="truncate text-body font-bold text-ink">
                            <Highlight
                              text={item.customer?.name || item.customer?.email || 'Unknown customer'}
                              query={debouncedQuery}
                            />
                          </p>
                          {item.customer?.email && (
                            <p className="truncate font-mono text-[11px] text-muted">
                              <Highlight text={item.customer.email} query={debouncedQuery} />
                            </p>
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
                          {item.order?.status && (
                            <div className="mt-1">
                              <StatusPill meta={statusMeta(item.order.status)} size="sm" withDot={false} />
                            </div>
                          )}
                        </td>
                        <td className="text-body text-ink">{formatCaseType(item.case_type)}</td>
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
                  to={`/merchant/cases/${item.id}`}
                  className={cn(
                    'lift block rounded border-2 border-line bg-surface p-4 shadow-hard',
                    needsAction(item.status) && 'border-l-[6px]',
                    item.status === 'ESCALATED' ? 'border-l-danger' : 'border-l-warning',
                  )}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="font-mono text-[13px] font-semibold text-ink">{item.case_number}</p>
                      <p className="mt-0.5 truncate text-meta text-muted">
                        {item.customer?.name || item.customer?.email || 'Unknown customer'}
                      </p>
                    </div>
                    <StatusPill meta={statusMeta(item.status)} size="sm" />
                  </div>
                  <p className="mt-2 text-meta font-bold text-ink">{formatCaseType(item.case_type)}</p>
                  <div className="mt-1.5 flex items-center justify-between gap-3 font-mono text-[11px] text-muted">
                    <span className="truncate">{item.order?.ref ?? '—'}</span>
                    <span>{formatTime(item.updated_at)}</span>
                  </div>
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
                title={cases.length === 0 ? 'No cases yet' : 'No cases match your filters'}
                description={
                  cases.length === 0
                    ? 'When a customer reports a payment issue, it appears here.'
                    : 'Try a different status, issue type or search term.'
                }
                action={
                  cases.length > 0 ? (
                    <Button variant="outline" size="sm" onClick={clearAll}>
                      Clear filters
                    </Button>
                  ) : undefined
                }
              />
            )}

            {filteredCases.length > 0 && (
              <div className="flex flex-col gap-3 border-t-2 border-line px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <p className="font-mono text-[12px] text-muted">
                  Showing {visibleCases.length} of {filteredCases.length}{' '}
                  {filteredCases.length === 1 ? 'case' : 'cases'}
                </p>
                <p className="hidden items-center gap-1.5 text-meta text-muted lg:flex">
                  <ArrowUpDown className="h-3.5 w-3.5" aria-hidden="true" strokeWidth={2.5} />
                  Needs attention first
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
                    <span className="font-mono text-[12px] text-muted">Page 1 of 1</span>
                  )}
                </div>
              </div>
            )}
          </div>

          {cases.length > 0 && (
            <div className="mt-auto flex items-center gap-2 rounded border-2 border-dashed border-line px-4 py-3 text-meta text-muted">
              <ArrowUpDown className="h-4 w-4 shrink-0" aria-hidden="true" strokeWidth={2.5} />
              Reconcile an order to settle it, or open a case to message the customer.
            </div>
          )}
        </div>

        <div className="list-rail">
          <InsightRail
            attention={attention}
            activity={activity}
            basePath="/merchant/cases"
            emptyAttention="Nothing needs your decision. New approval requests will appear here."
          />
        </div>
      </div>
    </div>
  );
}
