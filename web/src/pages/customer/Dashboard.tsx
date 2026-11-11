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
import { Tooltip } from '../../components/ui/Tooltip';
import { Highlight } from '../../components/ui/Highlight';
import { EmptyState, ErrorNotice, Skeleton } from '../../components/ui/States';

const TH = 'px-4 py-3 text-left text-meta font-medium text-muted';

export function CustomerDashboard() {
  const [cases, setCases] = useState<CaseSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [debouncedQuery, setDebouncedQuery] = useState('');
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

  const filteredCases = useMemo(() => {
    const needle = debouncedQuery.toLowerCase();
    if (!needle) return cases;
    return cases.filter((item) =>
      [item.case_number, item.merchant?.name, item.order?.ref, item.customer?.email]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(needle),
    );
  }, [cases, debouncedQuery]);

  if (loading) {
    return (
      <div className="space-y-5">
        <PageHeader title="My cases" description="Track payment issues and replies from your merchant." />
        <div className="rounded-lg border border-line bg-surface">
          <div className="border-b border-line px-4 py-3">
            <Skeleton className="h-4 w-full max-w-lg" />
          </div>
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="flex items-center gap-6 border-b border-line px-4 py-4 last:border-0">
              <Skeleton className="h-4 w-16" />
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-4 w-36" />
              <Skeleton className="ml-auto h-6 w-24 rounded-pill" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="space-y-5">
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
    <div className="space-y-5">
      <PageHeader
        title="My cases"
        description="Track payment issues and replies from your merchant."
        actions={
          <Button
            variant="primary"
            onClick={() => navigate('/customer/cases/new')}
            icon={<Plus className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
          >
            Report an issue
          </Button>
        }
      />

      {debouncedQuery && (
        <p className="text-meta text-muted">
          {filteredCases.length} {filteredCases.length === 1 ? 'result' : 'results'} for “{debouncedQuery}”
        </p>
      )}

      <div className="overflow-hidden rounded-lg border border-line bg-surface">
        <div className="hidden overflow-x-auto xl:block">
          <table className="w-full min-w-[950px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-muted">
                <th scope="col" className={cn(TH, 'w-[112px]')}>
                  Case
                </th>
                <th scope="col" className={cn(TH, 'min-w-[160px]')}>
                  Merchant
                </th>
                <th scope="col" className={cn(TH, 'w-[190px]')}>
                  Payment
                </th>
                <th scope="col" className={cn(TH, 'w-[232px]')}>
                  Order
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
                const href = `/customer/cases/${item.id}`;
                return (
                  <tr
                    key={item.id}
                    className="relative border-b border-line transition-colors duration-100 last:border-0 hover:bg-surface-muted"
                  >
                    <td className="px-4 py-3">
                      <Link
                        to={href}
                        aria-label={`Open case ${item.case_number}`}
                        className="absolute inset-0 z-10 rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue focus-visible:ring-inset"
                      />
                      <span className="text-body font-semibold text-ink">
                        <Highlight text={item.case_number} query={debouncedQuery} />
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <p className="truncate text-body text-ink">
                        <Highlight text={item.merchant?.name || 'Merchant'} query={debouncedQuery} />
                      </p>
                    </td>
                    <td className="px-4 py-3">
                      <p className="text-body font-medium tabular-nums text-ink">
                        {item.payment
                          ? formatMinorUnits(item.payment.amount_minor, item.payment.currency)
                          : '—'}
                      </p>
                      {item.payment && (
                        <div className="mt-1">
                          <StatusPill meta={statusMeta(item.payment.status)} size="sm" withDot={false} />
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <p className="break-all font-mono text-[12px] leading-5 text-ink">
                        <Highlight text={item.order?.ref ?? '—'} query={debouncedQuery} />
                      </p>
                      {item.order && (
                        <div className="mt-1">
                          <StatusPill meta={statusMeta(item.order.status)} size="sm" withDot={false} />
                        </div>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <StatusPill meta={statusMeta(item.status)} />
                    </td>
                    <td className="px-4 py-3 text-right">
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
              to={`/customer/cases/${item.id}`}
              className="block bg-surface px-4 py-3 transition-colors hover:bg-surface-muted"
            >
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-body font-semibold text-ink">{item.case_number}</p>
                  <p className="mt-0.5 text-meta text-muted">{item.merchant?.name || 'Merchant'}</p>
                </div>
                <StatusPill meta={statusMeta(item.status)} size="sm" />
              </div>
              <div className="mt-2 flex items-center justify-between gap-3 text-meta text-ink">
                <span className="tabular-nums">
                  {item.payment
                    ? formatMinorUnits(item.payment.amount_minor, item.payment.currency)
                    : '—'}
                </span>
                <span className="font-mono text-[12px]">{item.order?.ref ?? '—'}</span>
              </div>
              <p className="mt-1.5 text-[12px] text-muted">{formatTime(item.updated_at)}</p>
            </Link>
          ))}
          {filteredCases.length % 2 === 1 && (
            <div className="hidden bg-surface sm:block" aria-hidden="true" />
          )}
        </div>

        {filteredCases.length === 0 && (
          <EmptyState
            icon={cases.length === 0 ? <Inbox className="h-5 w-5" /> : <SearchX className="h-5 w-5" />}
            title={cases.length === 0 ? 'No cases yet' : 'No cases match your search'}
            description={
              cases.length === 0
                ? 'When a payment doesn’t match your order, report it here and we’ll help you sort it out.'
                : 'Try a different case number, merchant or order reference.'
            }
            action={
              cases.length === 0 ? (
                <Button
                  variant="outline"
                  onClick={() => navigate('/customer/cases/new')}
                  icon={<Plus className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
                >
                  Report an issue
                </Button>
              ) : (
                <Button variant="outline" onClick={clear}>
                  Clear search
                </Button>
              )
            }
          />
        )}
      </div>

      {cases.length > 0 && (
        <p className="flex items-center gap-1.5 text-[12px] text-muted">
          <ArrowUpRight className="h-3.5 w-3.5" aria-hidden="true" />
          Open a case to message your merchant or add evidence.
        </p>
      )}
    </div>
  );
}
