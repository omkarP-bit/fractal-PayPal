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
            className="relative flex flex-1 items-start gap-3 pb-5 last:pb-0 sm:flex-col sm:items-center sm:gap-0 sm:pb-0 sm:text-center"
          >
            {!last && (
              <span
                aria-hidden="true"
                className={cn(
                  'absolute bottom-0 left-[11px] top-6 w-0.5 rounded-pill sm:bottom-auto sm:left-1/2 sm:top-[11px] sm:h-0.5 sm:w-full',
                  nextReached ? 'bg-sky' : 'bg-line',
                )}
              />
            )}
            <span
              className={cn(
                'relative z-10 flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold',
                step.state === 'complete' && 'bg-blue text-white',
                step.state === 'current' &&
                  'motion-safe-pulse border-2 border-blue bg-surface text-blue ring-4 ring-info-bg',
                step.state === 'upcoming' && 'border border-line-strong bg-surface text-muted',
              )}
            >
              {step.state === 'complete' ? (
                <Check className="h-3.5 w-3.5" aria-hidden="true" />
              ) : (
                index + 1
              )}
            </span>
            <span
              className={cn(
                'pt-0.5 text-meta leading-5 sm:mt-2 sm:max-w-[124px] sm:pt-0',
                step.state === 'current' ? 'font-semibold text-ink' : 'text-muted',
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
