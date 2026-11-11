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
    'bg-yellow text-[color:var(--text-on-yellow)] hover:bg-yellow-hover shadow-none',
  secondary: 'bg-blue text-white hover:bg-blue-hover',
  outline: 'border border-line-strong bg-surface text-ink hover:bg-surface-muted',
  tertiary: 'text-blue hover:underline underline-offset-4',
  danger: 'border border-danger bg-transparent text-danger hover:bg-danger-bg',
  ghost: 'text-muted hover:bg-surface-muted hover:text-ink',
};

const SIZES: Record<Size, string> = {
  sm: 'h-9 px-3 text-meta',
  md: 'h-10 px-4 text-body',
  lg: 'h-11 px-6 text-body',
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
        'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-pill font-semibold transition-[background-color,color,transform,border-color] duration-150 active:scale-[0.98] disabled:pointer-events-none disabled:opacity-50',
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

/** A square, icon-only button. `label` becomes the accessible name and tooltip. */
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
        'inline-flex items-center justify-center rounded-pill transition-colors duration-150 active:scale-[0.97] disabled:pointer-events-none disabled:opacity-50',
        VARIANTS[variant],
        size === 'sm' ? 'h-9 w-9' : 'h-10 w-10',
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}
