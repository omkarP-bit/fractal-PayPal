import { useEffect, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';
import { cn } from '../../lib/cn';

export interface CopyChipProps {
  value: string;
  /** Text to display. Defaults to the first 8 characters of `value`. */
  label?: string;
  title?: string;
  className?: string;
  /** Hide the copy icon until the parent row is hovered or focused. */
  reveal?: boolean;
  /** Show only the copy icon; the value stays available to the accessible name. */
  iconOnly?: boolean;
}

export function CopyChip({
  value,
  label,
  title,
  className,
  reveal = false,
  iconOnly = false,
}: CopyChipProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (timer.current) window.clearTimeout(timer.current);
    },
    [],
  );

  const copy = async () => {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(value);
      } else {
        const helper = document.createElement('textarea');
        helper.value = value;
        helper.setAttribute('readonly', '');
        helper.style.position = 'fixed';
        helper.style.opacity = '0';
        document.body.appendChild(helper);
        helper.select();
        document.execCommand('copy');
        document.body.removeChild(helper);
      }
      setCopied(true);
      if (timer.current) window.clearTimeout(timer.current);
      timer.current = window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Copy is a convenience; the full value is still available to the user.
    }
  };

  const display = label ?? (value.length > 8 ? value.slice(0, 8) : value);
  const accessibleTitle = title ?? `Copy ID ${display}`;

  return (
    <span className={cn('inline-flex items-center', className)}>
      <button
        type="button"
        onClick={copy}
        title={accessibleTitle}
        aria-label={accessibleTitle}
        className="group inline-flex h-6 items-center gap-1.5 rounded-pill border border-line bg-surface-alt px-1.5 font-mono text-[12px] font-medium text-ink transition-colors duration-100 hover:bg-yellow"
      >
        {!iconOnly && <span>{display}</span>}
        {copied ? (
          <Check className="h-3 w-3 text-ink" aria-hidden="true" />
        ) : (
          <Copy
            className={cn(
              'h-3 w-3 text-ink transition-opacity duration-100',
              reveal
                ? 'opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100'
                : 'opacity-70 group-hover:opacity-100',
            )}
            aria-hidden="true"
          />
        )}
      </button>
      <span className="sr-only" aria-live="polite">
        {copied ? 'Copied' : ''}
      </span>
    </span>
  );
}
