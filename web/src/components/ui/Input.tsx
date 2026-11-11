import { useId } from 'react';
import type {
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from 'react';
import { AlertCircle, ChevronDown } from 'lucide-react';
import { cn } from '../../lib/cn';

const controlBase =
  'w-full rounded border-2 bg-surface text-body font-medium text-ink placeholder:text-muted/70 transition-[box-shadow,border-color] duration-100 focus:border-ink focus:shadow-[4px_4px_0_var(--brand-sky)] focus:outline-none disabled:cursor-not-allowed disabled:bg-surface-alt disabled:text-muted';

interface FieldShellProps {
  id: string;
  label?: string;
  hint?: string;
  error?: string;
  required?: boolean;
  children: ReactNode;
  className?: string;
}

function FieldShell({ id, label, hint, error, required, children, className }: FieldShellProps) {
  return (
    <div className={cn('space-y-1.5', className)}>
      {label && (
        <label htmlFor={id} className="block text-meta font-bold text-ink">
          {label}
          {required && (
            <span className="ml-1 text-[color:var(--danger-text)]" aria-hidden="true">
              *
            </span>
          )}
        </label>
      )}
      {children}
      {error ? (
        <p id={`${id}-error`} className="flex items-start gap-1.5 text-meta font-medium text-[color:var(--danger-text)]">
          <AlertCircle
            className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[color:var(--danger-text)]"
            aria-hidden="true"
          />
          <span>{error}</span>
        </p>
      ) : hint ? (
        <p id={`${id}-hint`} className="text-meta text-muted">
          {hint}
        </p>
      ) : null}
    </div>
  );
}

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  hint?: string;
  error?: string;
  leadingIcon?: ReactNode;
  trailing?: ReactNode;
  fieldClassName?: string;
}

export function Input({
  id,
  label,
  hint,
  error,
  leadingIcon,
  trailing,
  className,
  fieldClassName,
  required,
  ...props
}: InputProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const describedBy = error ? `${fieldId}-error` : hint ? `${fieldId}-hint` : undefined;

  return (
    <FieldShell
      id={fieldId}
      label={label}
      hint={hint}
      error={error}
      required={required}
      className={fieldClassName}
    >
      <div className="relative">
        {leadingIcon && (
          <span className="pointer-events-none absolute inset-y-0 left-3 flex items-center text-muted">
            {leadingIcon}
          </span>
        )}
        <input
          id={fieldId}
          required={required}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          className={cn(
            controlBase,
            'h-12 px-3',
            leadingIcon && 'pl-10',
            trailing && 'pr-10',
            error && 'border-[color:var(--danger)]',
            className,
          )}
          {...props}
        />
        {trailing && (
          <span className="absolute inset-y-0 right-2 flex items-center">{trailing}</span>
        )}
      </div>
    </FieldShell>
  );
}

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  hint?: string;
  error?: string;
  fieldClassName?: string;
}

export function Textarea({
  id,
  label,
  hint,
  error,
  className,
  fieldClassName,
  required,
  ...props
}: TextareaProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const describedBy = error ? `${fieldId}-error` : hint ? `${fieldId}-hint` : undefined;

  return (
    <FieldShell
      id={fieldId}
      label={label}
      hint={hint}
      error={error}
      required={required}
      className={fieldClassName}
    >
      <textarea
        id={fieldId}
        required={required}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy}
        className={cn(
          controlBase,
          'min-h-[120px] resize-y px-3 py-2.5',
          error && 'border-[color:var(--danger)]',
          className,
        )}
        {...props}
      />
    </FieldShell>
  );
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  hint?: string;
  error?: string;
  fieldClassName?: string;
  options: Array<{ value: string; label: string; disabled?: boolean }>;
}

export function Select({
  id,
  label,
  hint,
  error,
  options,
  className,
  fieldClassName,
  required,
  ...props
}: SelectProps) {
  const generatedId = useId();
  const fieldId = id ?? generatedId;
  const describedBy = error ? `${fieldId}-error` : hint ? `${fieldId}-hint` : undefined;

  return (
    <FieldShell
      id={fieldId}
      label={label}
      hint={hint}
      error={error}
      required={required}
      className={fieldClassName}
    >
      <div className="relative">
        <select
          id={fieldId}
          required={required}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy}
          className={cn(
            controlBase,
            'h-12 cursor-pointer appearance-none px-3 pr-9 font-bold',
            error && 'border-[color:var(--danger)]',
            className,
          )}
          {...props}
        >
          {options.map((option) => (
            <option key={option.value} value={option.value} disabled={option.disabled}>
              {option.label}
            </option>
          ))}
        </select>
        <ChevronDown
          className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink"
          aria-hidden="true"
        />
      </div>
    </FieldShell>
  );
}
