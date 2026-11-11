import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowUpDown, Inbox, SearchX, X } from 'lucide-react';
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
import { CopyChip } from '../../components/ui/CopyChip';
import { Tooltip } from '../../components/ui/Tooltip';
import { Highlight } from '../../components/ui/Highlight';
import { EmptyState, ErrorNotice, Skeleton } from '../../components/ui/States';

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

const TH = 'px-4 py-3 text-left text-meta font-medium text-muted';

export function MerchantDashboard() {
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [typeFilter, setTypeFilter] = useState('all');
  const [sort, setSort] = useState<SortOption>('attention');
  const [debouncedQuery, setDebouncedQuery] = useState('');
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
    { key: 'all' as const, label: 'Total cases', value: counts.all, dot: null },
    { key: 'needs-approval' as const, label: 'Needs approval', value: counts.needs, dot: 'warning' },
    { key: 'investigating' as const, label: 'Investigating', value: counts.investigating, dot: null },
    { key: 'escalated' as const, label: 'Escalated', value: counts.escalated, dot: 'danger' },
  ];

  if (loading) {
    return (
      <div className="space-y-6">
        <PageHeader title="Cases" description="Payment exceptions for your store." />
        <Skeleton className="h-[88px] w-full rounded-lg" />
        <div className="rounded-lg border border-line bg-surface">
          <div className="border-b border-line px-4 py-3">
            <Skeleton className="h-4 w-full max-w-xl" />
          </div>
          {Array.from({ length: 5 }).map((_, index) => (
            <div key={index} className="flex items-center gap-6 border-b border-line px-4 py-4 last:border-0">
              <Skeleton className="h-4 w-16" />
              <Skeleton className="h-4 w-40" />
              <Skeleton className="h-4 w-28" />
              <Skeleton className="h-4 w-40" />
              <Skeleton className="ml-auto h-6 w-24 rounded-pill" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-6">
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
    <div className="space-y-5">
      <PageHeader title="Cases" description="Payment exceptions for your store." />

      <div className="flex flex-col gap-3 xl:flex-row xl:items-center">
        <div className="grid flex-1 grid-cols-2 overflow-hidden rounded-lg border border-line bg-surface sm:flex">
          {summary.map((item, index) => {
            const active = statusFilter === item.key;
            return (
              <button
                key={item.key}
                type="button"
                onClick={() => setStatusFilter(active && item.key !== 'all' ? 'all' : item.key)}
                aria-pressed={active}
                aria-label={`Show ${item.label.toLowerCase()} (${item.value})`}
                className={cn(
                  'relative flex min-w-0 flex-1 flex-col items-start gap-0.5 px-3 py-2.5 text-left transition-colors duration-150 hover:bg-surface-muted sm:px-5 sm:py-4',
                  index % 2 === 1 && 'border-l border-line sm:border-l-0',
                  index >= 2 && 'border-t border-line sm:border-t-0',
                  active && 'bg-surface-muted',
                )}
              >
                {index > 0 && (
                  <span
                    className="absolute inset-y-3 left-0 hidden w-px bg-line sm:block"
                    aria-hidden="true"
                  />
                )}
                <span className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      'text-[22px] font-semibold leading-7 tabular-nums',
                      item.value === 0 ? 'text-muted' : 'text-ink',
                    )}
                  >
                    {item.value}
                  </span>
                  {item.dot && item.value > 0 && (
                    <span
                      className={cn(
                        'h-1.5 w-1.5 rounded-full',
                        item.dot === 'warning' ? 'bg-warning' : 'bg-danger',
                      )}
                      aria-hidden="true"
                    />
                  )}
                </span>
                <span className="truncate text-[12px] text-muted">{item.label}</span>
                {active && (
                  <span
                    className="absolute inset-x-0 bottom-0 h-0.5 bg-blue"
                    aria-hidden="true"
                  />
                )}
              </button>
            );
          })}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Select
            aria-label="Filter by status"
            value={statusFilter}
            onChange={(event) => setStatusFilter(event.target.value as StatusFilter)}
            options={STATUS_OPTIONS}
            className="h-9 w-auto min-w-[136px] text-meta"
          />
          <Select
            aria-label="Filter by issue type"
            value={typeFilter}
            onChange={(event) => setTypeFilter(event.target.value)}
            options={typeOptions}
            className="h-9 w-auto min-w-[136px] text-meta"
          />
          <Select
            aria-label="Sort cases"
            value={sort}
            onChange={(event) => setSort(event.target.value as SortOption)}
            options={SORT_OPTIONS}
            className="h-9 w-auto min-w-[168px] text-meta"
          />
        </div>
      </div>

      {activeFilters.length > 0 && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1 text-[12px] text-muted">
            <ArrowUpDown className="h-3.5 w-3.5" aria-hidden="true" />
            Filtered
          </span>
          {activeFilters.map((filter) => (
            <span
              key={filter.key}
              className="inline-flex max-w-full items-center gap-1 rounded-pill bg-info-bg py-0.5 pl-2.5 pr-0.5 text-[12px] text-info"
            >
              <span className="truncate">{filter.label}</span>
              <IconButton
                label={`Remove ${filter.key} filter`}
                size="sm"
                onClick={filter.remove}
                className="h-6 w-6 hover:bg-surface"
              >
                <X className="h-3 w-3" aria-hidden="true" />
              </IconButton>
            </span>
          ))}
          <Button variant="tertiary" size="sm" onClick={clearAll} className="h-7 px-1.5">
            Clear all
          </Button>
        </div>
      )}

      <div className="overflow-hidden rounded-lg border border-line bg-surface">
        <div className="hidden overflow-x-auto xl:block">
          <table className="w-full min-w-[980px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-muted">
                <th scope="col" className={cn(TH, 'w-[112px]')}>
                  Case
                </th>
                <th scope="col" className={cn(TH, 'min-w-[190px]')}>
                  Customer
                </th>
                <th scope="col" className={cn(TH, 'w-[232px]')}>
                  Order
                </th>
                <th scope="col" className={cn(TH, 'min-w-[180px]')}>
                  Issue
                </th>
                <th scope="col" className={cn(TH, 'w-[148px]')}>
                  Status
                </th>
                <th scope="col" className={cn(TH, 'w-[110px] text-right')}>
                  Updated
                </th>
              </tr>
            </thead>
            <tbody>
              {filteredCases.map((item) => {
                const action = needsAction(item.status);
                const href = `/merchant/cases/${item.id}`;
                return (
                  <tr
                    key={item.id}
                    className="group/row relative border-b border-line transition-colors duration-100 last:border-0 hover:bg-surface-muted"
                  >
                    <td className="px-4 py-2.5">
                      {action && (
                        <span
                          className={cn(
                            'absolute inset-y-0 left-0 w-[3px]',
                            item.status === 'ESCALATED' ? 'bg-danger' : 'bg-warning',
                          )}
                          aria-hidden="true"
                        />
                      )}
                      <Link
                        to={href}
                        aria-label={`Open case ${item.case_number}`}
                        className="absolute inset-0 z-10 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue focus-visible:ring-inset"
                      />
                      <span className="text-body font-semibold text-ink">{item.case_number}</span>
                    </td>
                    <td className="px-4 py-2.5">
                      <p className="truncate text-body font-medium text-ink">
                        <Highlight
                          text={item.customer?.name || item.customer?.email || 'Unknown customer'}
                          query={debouncedQuery}
                        />
                      </p>
                      {item.customer?.email && (
                        <p className="truncate text-[12px] text-muted">
                          <Highlight text={item.customer.email} query={debouncedQuery} />
                        </p>
                      )}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-start gap-1">
                        <span className="relative z-20 min-w-0 break-all font-mono text-[12px] leading-5 text-ink">
                          <Highlight text={item.order?.ref ?? '—'} query={debouncedQuery} />
                        </span>
                        {item.order?.ref && (
                          <span className="relative z-20 shrink-0">
                            <CopyChip
                              value={item.order.ref}
                              title={`Copy order ${item.order.ref}`}
                              iconOnly
                              reveal
                              className="[&>button]:h-5 [&>button]:bg-transparent [&>button]:px-1"
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
                    <td className="px-4 py-2.5 text-body text-ink">
                      {formatCaseType(item.case_type)}
                    </td>
                    <td className="px-4 py-2.5">
                      <StatusPill meta={statusMeta(item.status)} />
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <Tooltip content={formatAbsolute(item.updated_at)} side="left">
                        <time dateTime={item.updated_at} className="text-[12px] text-muted">
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

        <div className="grid gap-px bg-line sm:grid-cols-2 xl:hidden">
          {filteredCases.map((item) => (
            <Link
              key={item.id}
              to={`/merchant/cases/${item.id}`}
              className={cn(
                'block bg-surface px-4 py-3 transition-colors hover:bg-surface-muted',
                needsAction(item.status) && 'border-l-[3px] border-l-warning',
              )}
            >
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-body font-semibold text-ink">{item.case_number}</p>
                  <p className="mt-0.5 text-meta text-muted">
                    {item.customer?.name || item.customer?.email || 'Unknown customer'}
                  </p>
                </div>
                <StatusPill meta={statusMeta(item.status)} size="sm" />
              </div>
              <p className="mt-2 text-meta text-ink">{formatCaseType(item.case_type)}</p>
              <div className="mt-1.5 flex items-center justify-between gap-3 text-[12px] text-muted">
                <span className="font-mono">{item.order?.ref ?? '—'}</span>
                <span>{formatTime(item.updated_at)}</span>
              </div>
            </Link>
          ))}
          {filteredCases.length % 2 === 1 && (
            <div className="hidden bg-surface sm:block" aria-hidden="true" />
          )}
        </div>

        {filteredCases.length === 0 && (
          <EmptyState
            icon={cases.length === 0 ? <Inbox className="h-5 w-5" /> : <SearchX className="h-5 w-5" />}
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
      </div>
    </div>
  );
}
