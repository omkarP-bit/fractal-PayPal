import type { ReactNode } from 'react';
import { cn } from '../../lib/cn';

export function Panel({
  children,
  className,
  flush = false,
  as: Tag = 'section',
}: {
  children: ReactNode;
  className?: string;
  flush?: boolean;
  as?: 'section' | 'div' | 'article' | 'aside';
}) {
  return (
    <Tag className={cn('panel', !flush && 'p-6', className)}>
      {children}
    </Tag>
  );
}

export function PanelHeader({
  title,
  description,
  aside,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  aside?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('flex items-start justify-between gap-4', className)}>
      <div className="min-w-0">
        <h2 className="text-section font-bold text-ink">{title}</h2>
        {description && <p className="mt-1 text-meta text-muted">{description}</p>}
      </div>
      {aside && <div className="shrink-0">{aside}</div>}
    </div>
  );
}

export function Divider({ className }: { className?: string }) {
  return <hr className={cn('border-0 border-t-2 border-line', className)} />;
}

export function PageHeader({
  title,
  description,
  actions,
  className,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between',
        className,
      )}
    >
      <div className="min-w-0">
        <h1 className="text-title font-extrabold text-ink">{title}</h1>
        {description && <p className="mt-1 max-w-prose text-body text-muted">{description}</p>}
      </div>
      {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
    </div>
  );
}
