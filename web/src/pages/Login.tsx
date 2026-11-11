import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowRight, Mail, Store, UserRound } from 'lucide-react';
import { useAuthStore } from '../store/auth';
import { apiClient } from '../services/api';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';
import { ErrorNotice } from '../components/ui/States';
import { cn } from '../lib/cn';
import { apiErrorMessage } from '../lib/format';

type Role = 'CUSTOMER' | 'MERCHANT';

const ROLES: Array<{
  value: Role;
  label: string;
  description: string;
  icon: typeof UserRound;
}> = [
  {
    value: 'CUSTOMER',
    label: 'Customer',
    description: 'Report and follow a payment issue',
    icon: UserRound,
  },
  {
    value: 'MERCHANT',
    label: 'Merchant',
    description: 'Review and resolve store cases',
    icon: Store,
  },
];

const DEMO_ACCOUNTS: Array<{ role: Role; label: string; email: string; icon: typeof UserRound }> = [
  { role: 'CUSTOMER', label: 'Customer', email: 'buyer@example.com', icon: UserRound },
  { role: 'MERCHANT', label: 'Merchant', email: 'ops@acme.com', icon: Store },
];

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function LoginPage() {
  const [role, setRole] = useState<Role>('CUSTOMER');
  const [email, setEmail] = useState('');
  const [touched, setTouched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const login = useAuthStore((state) => state.login);
  const navigate = useNavigate();

  const emailError = touched && !EMAIL_PATTERN.test(email.trim()) ? 'Enter a valid email address.' : '';

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setTouched(true);
    setError('');
    if (!EMAIL_PATTERN.test(email.trim())) return;

    setLoading(true);
    try {
      const session = await apiClient.devLogin(role, email.trim());
      login(session.token, session.user);
      navigate(role === 'MERCHANT' ? '/merchant/cases' : '/customer/cases');
    } catch (err) {
      setError(apiErrorMessage(err, 'We could not sign you in. Check your details and try again.'));
    } finally {
      setLoading(false);
    }
  };

  const fillDemo = (account: (typeof DEMO_ACCOUNTS)[number]) => {
    setRole(account.role);
    setEmail(account.email);
    setTouched(false);
    setError('');
  };

  return (
    <div className="flex min-h-screen flex-col items-center justify-center px-4 py-12">
      <main className="w-full max-w-[400px]">
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="mb-4 flex h-11 w-11 items-center justify-center rounded-lg bg-navy text-[19px] font-semibold text-[color:var(--surface)]">
            F
          </span>
          <h1 className="text-title font-semibold text-ink">Sign in to FRACTAL</h1>
          <p className="mt-1.5 text-body text-muted">
            Resolve payment exceptions with confidence.
          </p>
        </div>

        <div className="rounded-lg border border-line bg-surface p-6 sm:p-8">
          <form onSubmit={handleSubmit} className="space-y-5" noValidate>
            <fieldset>
              <legend className="mb-2 block text-meta font-medium text-ink">I am a</legend>
              <div
                role="group"
                aria-label="Account type"
                className="relative grid grid-cols-2 gap-1 rounded-md bg-surface-muted p-1"
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'pointer-events-none absolute inset-y-1 left-1 w-[calc(50%-4px)] rounded-sm bg-surface shadow-sm transition-transform duration-150 ease-out',
                    role === 'MERCHANT' && 'translate-x-full',
                  )}
                />
                {ROLES.map((option) => {
                  const selected = role === option.value;
                  return (
                    <button
                      key={option.value}
                      type="button"
                      aria-pressed={selected}
                      onClick={() => setRole(option.value)}
                      className={cn(
                        'relative z-10 flex flex-col items-center gap-0.5 rounded-sm px-2 py-2.5 text-center transition-colors duration-150',
                        selected ? 'text-ink' : 'text-muted hover:text-ink',
                      )}
                    >
                      <span className="flex items-center gap-1.5 text-body font-medium">
                        <option.icon className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />
                        {option.label}
                      </span>
                      <span className="text-[11px] leading-4 text-muted">{option.description}</span>
                    </button>
                  );
                })}
              </div>
            </fieldset>

            <Input
              label="Email address"
              type="email"
              name="email"
              autoComplete="email"
              inputMode="email"
              placeholder="you@example.com"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              onBlur={() => setTouched(true)}
              error={emailError}
              required
              leadingIcon={<Mail className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
            />

            {error && <ErrorNotice title="Sign-in failed" description={error} />}

            <Button
              type="submit"
              variant="primary"
              size="lg"
              className="w-full"
              isLoading={loading}
              loadingText="Signing in…"
            >
              Continue
              <ArrowRight className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />
            </Button>
          </form>

          <div className="mt-6 border-t border-line pt-4">
            <p className="text-meta font-medium text-ink">Demo credentials</p>
            <p className="mt-0.5 text-[11px] text-muted">Click one to fill the form.</p>
            <div className="mt-2.5 flex flex-wrap gap-2">
              {DEMO_ACCOUNTS.map((account) => (
                <button
                  key={account.email}
                  type="button"
                  onClick={() => fillDemo(account)}
                  className="inline-flex items-center gap-1.5 rounded-pill border border-line bg-surface px-2.5 py-1.5 text-[12px] text-ink transition-colors duration-150 hover:border-line-strong hover:bg-surface-muted"
                >
                  <account.icon className="h-3.5 w-3.5 text-muted" aria-hidden="true" strokeWidth={1.75} />
                  <span className="font-medium">{account.label}</span>
                  <span className="font-mono text-[11px] text-muted">{account.email}</span>
                </button>
              ))}
            </div>
          </div>
        </div>

        <p className="mt-5 text-center text-[11px] text-muted">
          Dev mode: no password required.
        </p>
      </main>
    </div>
  );
}
