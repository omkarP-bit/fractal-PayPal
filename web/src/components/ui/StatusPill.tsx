import type { ReactNode } from 'react';
import { cn } from '../../lib/cn';
import type { StatusMeta, Tone } from '../../lib/format';

export const TONE_SURFACE: Record<Tone, string> = {
  success: 'bg-success-bg text-success',
  warning: 'bg-warning-bg text-warning',
  danger: 'bg-danger-bg text-danger',
  info: 'bg-info-bg text-info',
  neutral: 'bg-neutral-bg text-neutral',
};

export const TONE_TEXT: Record<Tone, string> = {
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
  info: 'text-info',
  neutral: 'text-neutral',
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

/** 24px tinted pill with a dot and a text label — never color-only. */
export function StatusPill({ meta, size = 'md', className, withDot = true }: StatusPillProps) {
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center gap-1.5 rounded-pill font-medium',
        size === 'sm' ? 'h-5 px-2 text-[11px]' : 'h-6 px-2.5 text-[12px]',
        TONE_SURFACE[meta.tone],
        className,
      )}
    >
      {withDot && (
        <span className={cn('h-1.5 w-1.5 rounded-full', TONE_DOT[meta.tone])} aria-hidden="true" />
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
        'inline-flex items-center gap-1 rounded-pill px-2 py-0.5 text-[11px] font-medium',
        TONE_SURFACE[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}
