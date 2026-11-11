import type { ReactNode } from 'react';
import { cn } from '../../lib/cn';

export function Tooltip({
  content,
  children,
  side = 'top',
  className,
}: {
  content: ReactNode;
  children: ReactNode;
  side?: 'top' | 'bottom' | 'left';
  className?: string;
}) {
  const position =
    side === 'bottom'
      ? 'left-1/2 top-full mt-2 -translate-x-1/2'
      : side === 'left'
        ? 'right-full top-1/2 mr-2 -translate-y-1/2'
        : 'bottom-full left-1/2 mb-2 -translate-x-1/2';

  return (
    <span
      className={cn('group/tooltip relative inline-flex', className)}
      title={typeof content === 'string' ? content : undefined}
    >
      {children}
      <span
        role="tooltip"
        aria-hidden="true"
        className={cn(
          'pointer-events-none absolute z-50 w-max max-w-[240px] rounded-sm bg-ink px-2 py-1 text-[12px] font-normal leading-4 text-surface opacity-0 shadow-pop transition-opacity duration-150 group-hover/tooltip:opacity-100 group-focus-within/tooltip:opacity-100',
          position,
        )}
      >
        {content}
      </span>
    </span>
  );
}
