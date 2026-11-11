import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { Loader2 } from 'lucide-react';
import { cn } from '../../lib/cn';

type Variant = 'primary' | 'secondary' | 'outline' | 'tertiary' | 'danger' | 'ghost';
type Size = 'sm' | 'md' | 'lg';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  isLoading?: boolean;
  loadingText?: string;
  icon?: ReactNode;
}

const VARIANTS: Record<Variant, string> = {
  primary:
    'btn-lift bg-yellow text-[color:var(--text-on-yellow)] hover:bg-yellow-hover disabled:bg-[color:var(--neutral)] disabled:text-[color:var(--neutral-ink)] disabled:border-dashed',
  secondary:
    'btn-lift bg-blue text-[color:var(--text-on-blue)] hover:bg-blue-hover disabled:bg-[color:var(--neutral)] disabled:text-[color:var(--neutral-ink)] disabled:border-dashed',
  outline:
    'btn-lift bg-surface text-ink hover:bg-surface-alt disabled:bg-surface-alt disabled:text-muted disabled:border-dashed',
  tertiary:
    'btn-flat border-2 border-transparent bg-transparent text-blue shadow-none hover:underline hover:underline-offset-4 disabled:text-muted',
  danger:
    'btn-lift bg-danger text-[color:var(--danger-ink)] hover:bg-[color:var(--danger)] disabled:opacity-60',
  ghost:
    'btn-flat border-2 border-transparent bg-transparent text-ink shadow-none hover:bg-surface-alt disabled:text-muted',
};

const SIZES: Record<Size, string> = {
  sm: 'h-9 px-3 text-meta',
  md: 'h-11 px-5 text-body',
  lg: 'h-12 px-6 text-body',
};

export function Button({
  className,
  variant = 'primary',
  size = 'md',
  isLoading = false,
  loadingText,
  icon,
  children,
  disabled,
  type = 'button',
  ...props
}: ButtonProps) {
  return (
    <button
      type={type}
      className={cn(
        'btn inline-flex select-none items-center justify-center gap-2 whitespace-nowrap rounded font-extrabold',
        'disabled:pointer-events-none disabled:shadow-none',
        VARIANTS[variant],
        SIZES[size],
        className,
      )}
      disabled={disabled || isLoading}
      aria-busy={isLoading || undefined}
      {...props}
    >
      {isLoading ? (
        <>
          <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          <span>{loadingText ?? children}</span>
        </>
      ) : (
        <>
          {icon}
          {children}
        </>
      )}
    </button>
  );
}

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  variant?: Variant;
  size?: 'sm' | 'md';
}

/** A 44px square, icon-only button. `label` is the accessible name and title. */
export function IconButton({
  label,
  className,
  variant = 'ghost',
  size = 'md',
  children,
  type = 'button',
  ...props
}: IconButtonProps) {
  return (
    <button
      type={type}
      aria-label={label}
      title={label}
      className={cn(
        'btn inline-flex shrink-0 items-center justify-center rounded',
        'disabled:pointer-events-none disabled:shadow-none',
        VARIANTS[variant],
        'border-2 border-line bg-surface text-ink shadow-hard hover:bg-surface-alt',
        size === 'sm' ? 'h-11 w-11' : 'h-11 w-11',
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}
