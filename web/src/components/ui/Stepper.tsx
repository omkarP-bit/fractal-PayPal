import { Check } from 'lucide-react';
import { cn } from '../../lib/cn';

export type StepState = 'complete' | 'current' | 'upcoming';

export interface Step {
  label: string;
  state: StepState;
}

export function Stepper({ steps, className }: { steps: Step[]; className?: string }) {
  return (
    <ol className={cn('flex flex-col sm:flex-row sm:items-start', className)}>
      {steps.map((step, index) => {
        const last = index === steps.length - 1;
        const nextReached = !last && steps[index + 1]?.state !== 'upcoming';

        return (
          <li
            key={step.label}
            className="relative flex flex-1 items-start gap-3 pb-7 last:pb-0 sm:flex-col sm:items-center sm:gap-0 sm:pb-0 sm:text-center"
          >
            {!last && (
              <span
                aria-hidden="true"
                className={cn(
                  'absolute bottom-0 left-[19px] top-10 w-1 sm:bottom-auto sm:left-1/2 sm:top-5 sm:h-1 sm:w-full sm:-translate-y-1/2',
                  nextReached ? 'bg-blue' : 'bg-line',
                )}
              />
            )}
            <span
              className={cn(
                'relative z-10 flex h-10 w-10 shrink-0 items-center justify-center rounded border-2 border-line text-[15px] font-extrabold',
                step.state === 'complete' && 'bg-blue text-[color:var(--text-on-blue)]',
                step.state === 'current' &&
                  'bg-yellow text-[color:var(--text-on-yellow)] shadow-hard',
                step.state === 'upcoming' && 'border-dashed bg-surface text-muted',
              )}
            >
              {step.state === 'complete' ? (
                <Check className="h-5 w-5" aria-hidden="true" strokeWidth={3} />
              ) : (
                index + 1
              )}
            </span>
            <span
              className={cn(
                'pt-1 text-meta leading-5 sm:mt-3 sm:max-w-[132px] sm:pt-0',
                step.state === 'current' ? 'font-bold text-ink' : 'text-muted',
              )}
            >
              {step.label}
              <span className="sr-only">
                {step.state === 'complete'
                  ? ' — completed'
                  : step.state === 'current'
                    ? ' — current step'
                    : ' — not started'}
              </span>
            </span>
          </li>
        );
      })}
    </ol>
  );
}
