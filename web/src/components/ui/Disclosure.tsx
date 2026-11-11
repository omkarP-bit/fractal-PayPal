import { useId, useState, type ReactNode } from 'react';
import { ChevronDown } from 'lucide-react';
import { cn } from '../../lib/cn';

export interface DisclosureProps {
  title: ReactNode;
  meta?: ReactNode;
  children: ReactNode;
  defaultOpen?: boolean;
  className?: string;
  contentClassName?: string;
  /** Optional controlled state. */
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

/** Height-animated disclosure used for evidence and permissions. */
export function Disclosure({
  title,
  meta,
  children,
  defaultOpen = false,
  className,
  contentClassName,
  open: controlledOpen,
  onOpenChange,
}: DisclosureProps) {
  const [internalOpen, setInternalOpen] = useState(defaultOpen);
  const open = controlledOpen ?? internalOpen;
  const id = useId();

  const toggle = () => {
    const next = !open;
    setInternalOpen(next);
    onOpenChange?.(next);
  };

  return (
    <div className={cn('w-full', className)}>
      <button
        type="button"
        onClick={toggle}
        aria-expanded={open}
        aria-controls={id}
        className="flex w-full items-center justify-between gap-3 rounded-sm text-left focus-visible:outline-offset-4"
      >
        <span className="flex min-w-0 items-center gap-2 text-body font-medium text-ink">{title}</span>
        <span className="flex shrink-0 items-center gap-2 text-meta text-muted">
          {meta}
          <ChevronDown
            className={cn('h-4 w-4 transition-transform duration-200 ease-out', open && 'rotate-180')}
            aria-hidden="true"
          />
        </span>
      </button>
      <div
        className={cn(
          'grid transition-[grid-template-rows] duration-200 ease-out',
          open ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]',
        )}
      >
        <div className="overflow-hidden">
          <div id={id} className={cn('pt-3', contentClassName)}>
            {children}
          </div>
        </div>
      </div>
    </div>
  );
}
