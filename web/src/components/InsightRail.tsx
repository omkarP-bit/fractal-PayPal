import { Link } from 'react-router-dom';
import type { ReactNode } from 'react';
import { Activity, AlertTriangle } from 'lucide-react';
import { formatAbsolute, formatTime, statusMeta } from '../lib/format';
import { cn } from '../lib/cn';
import { StatusPill } from './ui/StatusPill';
import { Tooltip } from './ui/Tooltip';

export interface RailItem {
  id: string;
  caseNumber: string;
  status: string;
  meta?: string;
  updatedAt: string;
}

function RailPanel({
  icon,
  title,
  empty,
  children,
}: {
  icon: ReactNode;
  title: string;
  empty: string;
  children: ReactNode;
}) {
  return (
    <section className="panel-flush overflow-hidden">
      <div className="rail-head flex items-center gap-2">
        {icon}
        <h2 className="text-[12px] font-bold uppercase tracking-[0.04em]">{title}</h2>
      </div>
      {children || (
        <p className="px-4 py-4 text-meta text-muted">{empty}</p>
      )}
    </section>
  );
}

/**
 * Large-screen information rail. It renders at every width but only sits
 * beside the table at 1920px and up; below that it stacks underneath.
 */
export function InsightRail({
  attention,
  activity,
  attentionTitle = 'Needs your attention',
  emptyAttention = 'Nothing needs your attention right now.',
  basePath,
}: {
  attention: RailItem[];
  activity: RailItem[];
  attentionTitle?: string;
  emptyAttention?: string;
  basePath: string;
}) {
  return (
    <aside className="min-w-0 space-y-5" aria-label="Case insights">
      <RailPanel
        icon={<AlertTriangle className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
        title={attentionTitle}
        empty={emptyAttention}
      >
        {attention.length > 0 && (
          <ul>
            {attention.map((item) => (
              <li key={item.id} className="border-b-2 border-line last:border-b-0">
                <Link
                  to={`${basePath}/${item.id}`}
                  className="block px-4 py-3 transition-colors duration-100 hover:bg-[color-mix(in_srgb,var(--brand-yellow)_35%,var(--surface))]"
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-[12px] font-semibold text-ink">
                      {item.caseNumber}
                    </span>
                    <StatusPill meta={statusMeta(item.status)} size="sm" />
                  </div>
                  {item.meta && (
                    <p className="mt-1 truncate text-meta text-muted">{item.meta}</p>
                  )}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </RailPanel>

      <RailPanel
        icon={<Activity className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
        title="Recent activity"
        empty="No activity has been recorded yet."
      >
        {activity.length > 0 && (
          <ul>
            {activity.map((item, index) => (
              <li
                key={`${item.id}-${index}`}
                className={cn(
                  'border-b-2 border-line last:border-b-0',
                  index === 0 && 'bg-surface-alt',
                )}
              >
                <Link to={`${basePath}/${item.id}`} className="block px-4 py-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-[12px] font-semibold text-ink">
                      {item.caseNumber}
                    </span>
                    <Tooltip content={formatAbsolute(item.updatedAt)} side="left">
                      <time
                        dateTime={item.updatedAt}
                        className="font-mono text-[11px] text-muted"
                      >
                        {formatTime(item.updatedAt)}
                      </time>
                    </Tooltip>
                  </div>
                  <p className="mt-1 truncate text-meta text-muted">
                    {item.meta || statusMeta(item.status).label}
                  </p>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </RailPanel>
    </aside>
  );
}
