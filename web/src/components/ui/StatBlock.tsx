import { cn } from '../../lib/cn';

export interface StatBlockProps {
  label: string;
  value: number;
  active?: boolean;
  onClick?: () => void;
  /** Flat fill for a block that carries urgency. */
  tone?: 'neutral' | 'warning' | 'danger' | 'success' | 'info';
  className?: string;
}

const TONE_FILL: Record<NonNullable<StatBlockProps['tone']>, string> = {
  neutral: 'bg-surface text-ink',
  warning: 'bg-warning text-[color:var(--warning-ink)]',
  danger: 'bg-danger text-[color:var(--danger-ink)]',
  success: 'bg-success text-[color:var(--success-ink)]',
  info: 'bg-info text-[color:var(--info-ink)]',
};

/**
 * A clickable summary block. Zero counts stay visible but read as inactive:
 * dashed outline, muted text and no shadow.
 */
export function StatBlock({
  label,
  value,
  active = false,
  onClick,
  tone = 'neutral',
  className,
}: StatBlockProps) {
  const empty = value === 0;

  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        'relative flex min-w-0 flex-col items-start gap-0.5 rounded border-2 p-3 text-left',
        empty
          ? 'border-dashed bg-surface text-muted shadow-none'
          : cn(TONE_FILL[tone], 'border-line shadow-hard'),
        onClick && !empty && 'lift',
        active &&
          'outline outline-[3px] outline-offset-2 outline-[color:var(--brand-sky)]',
        'min-h-[84px]',
        className,
      )}
    >
      <span className="text-[28px] font-extrabold leading-8 tabular-nums lg:text-[32px] lg:leading-9">
        {value}
      </span>
      <span className="truncate text-[11px] font-bold uppercase tracking-[0.04em]">
        {label}
      </span>
      {active && (
        <span
          className="absolute right-2 top-2 h-2 w-2 bg-[color:var(--ink)]"
          aria-hidden="true"
        />
      )}
    </button>
  );
}
