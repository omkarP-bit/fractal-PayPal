import type { ReactNode } from 'react';
import { cn } from '../../lib/cn';
import type { StatusMeta, Tone } from '../../lib/format';

/** Flat status fills. Text always uses the matching *-ink token. */
export const TONE_SURFACE: Record<Tone, string> = {
  success: 'bg-success text-[color:var(--success-ink)]',
  warning: 'bg-warning text-[color:var(--warning-ink)]',
  danger: 'bg-danger text-[color:var(--danger-ink)]',
  info: 'bg-info text-[color:var(--info-ink)]',
  neutral: 'bg-neutral text-[color:var(--neutral-ink)]',
};

export const TONE_TEXT: Record<Tone, string> = {
  success: 'text-[color:var(--success-ink)]',
  warning: 'text-[color:var(--warning-ink)]',
  danger: 'text-[color:var(--danger-ink)]',
  info: 'text-[color:var(--info-ink)]',
  neutral: 'text-[color:var(--neutral-ink)]',
};

export const TONE_DOT: Record<Tone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  info: 'bg-info',
  neutral: 'bg-neutral',
};

export interface StatusPillProps {
  meta: StatusMeta;
  size?: 'sm' | 'md';
  className?: string;
  withDot?: boolean;
}

/** A flat, outlined status tag with a square dot and an uppercase label. */
export function StatusPill({ meta, size = 'md', className, withDot = true }: StatusPillProps) {
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap rounded-pill border-2 border-line font-bold uppercase tracking-[0.04em]',
        size === 'sm' ? 'h-[22px] px-1.5 text-[11px]' : 'h-[26px] px-2 text-[12px]',
        TONE_SURFACE[meta.tone],
        className,
      )}
    >
      {withDot && (
        <span
          className="h-2 w-2 shrink-0 bg-[color:var(--ink)]"
          aria-hidden="true"
        />
      )}
      {meta.label}
    </span>
  );
}

export function Tag({
  children,
  tone = 'neutral',
  className,
}: {
  children: ReactNode;
  tone?: Tone;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-pill border-2 border-line px-1.5 py-0.5 text-[12px] font-bold uppercase tracking-[0.04em]',
        TONE_SURFACE[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
