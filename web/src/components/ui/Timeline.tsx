import { Lock } from 'lucide-react';
import { cn } from '../../lib/cn';
import { formatAbsolute, formatTime } from '../../lib/format';
import { Tooltip } from './Tooltip';

export interface TimelineEntry {
  id: string | number;
  time: string;
  title: string;
  detail?: string;
  actor?: string;
  customerVisible?: boolean;
}

export function Timeline({
  entries,
  showMerchantOnly = true,
  emptyText = 'No activity has been recorded yet.',
  className,
}: {
  entries: TimelineEntry[];
  showMerchantOnly?: boolean;
  emptyText?: string;
  className?: string;
}) {
  const visible = showMerchantOnly ? entries : entries.filter((entry) => entry.customerVisible);

  if (visible.length === 0) {
    return <p className="text-body text-muted">{emptyText}</p>;
  }

  return (
    <ol className={cn('space-y-0', className)}>
      {visible.map((entry, index) => (
        <li key={entry.id} className="grid grid-cols-[64px_12px_1fr] gap-x-2.5">
          <Tooltip content={formatAbsolute(entry.time)} side="left" className="justify-end">
            <time
              dateTime={entry.time}
              className="whitespace-nowrap pt-0.5 text-right text-[11px] leading-4 text-muted"
            >
              {formatTime(entry.time)}
            </time>
          </Tooltip>
          <div className="flex flex-col items-center">
            <span
              className={cn(
                'mt-1.5 h-2 w-2 shrink-0 rounded-full',
                entry.customerVisible ? 'bg-blue' : 'bg-neutral',
              )}
              aria-hidden="true"
            />
            {index < visible.length - 1 && (
              <span className="mt-1 w-px flex-1 bg-line" aria-hidden="true" />
            )}
          </div>
          <div className={cn('min-w-0', index < visible.length - 1 && 'pb-5')}>
            <p className="text-body text-ink">{entry.title}</p>
            <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px] leading-4 text-muted">
              {entry.actor && <span>{entry.actor}</span>}
              {!entry.customerVisible && (
                <span className="inline-flex items-center gap-1">
                  {entry.actor && <span aria-hidden="true">·</span>}
                  <Lock className="h-3 w-3" aria-hidden="true" />
                  Merchant only
                </span>
              )}
            </p>
            {entry.detail && <p className="mt-1 text-meta text-muted">{entry.detail}</p>}
          </div>
        </li>
      ))}
    </ol>
  );
}
