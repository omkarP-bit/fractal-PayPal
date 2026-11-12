import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowRight, LockKeyhole, Mail, Store, UserRound } from 'lucide-react';
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
  const [password, setPassword] = useState('');
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
      const session = password.trim()
        ? await apiClient.loginWithPassword(email.trim(), password)
        : await apiClient.devLogin(role, email.trim());
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
    <div className="flex min-h-screen flex-col items-center justify-center bg-bg px-4 py-12">
      <main className="w-full max-w-[440px]">
        <div className="mb-8 flex flex-col items-center text-center">
          <span className="mb-4 flex h-12 w-12 items-center justify-center rounded border-2 border-line bg-[#003087] text-[22px] font-extrabold text-white shadow-hard">
            F
          </span>
          <h1 className="text-title font-extrabold text-ink">Sign in to FRACTAL</h1>
          <p className="mt-1.5 text-body text-muted">
            Resolve payment exceptions with confidence.
          </p>
        </div>

        <div className="brut-panel w-full rounded border-2 border-line bg-surface p-6 shadow-hard-lg sm:p-8">
          <form onSubmit={handleSubmit} className="space-y-5" noValidate>
            <fieldset>
              <legend className="mb-2 block text-meta font-bold text-ink">I am a</legend>
              <div role="group" aria-label="Account type" className="grid grid-cols-2 gap-3">
                {ROLES.map((option) => {
                  const selected = role === option.value;
                  return (
                    <button
                      key={option.value}
                      type="button"
                      aria-pressed={selected}
                      onClick={() => setRole(option.value)}
                      className={cn(
                        'flex min-h-[92px] flex-col items-center justify-center gap-1 rounded border-2 border-line px-2 py-3 text-center transition-[background-color,box-shadow,transform] duration-100',
                        selected
                          ? 'bg-yellow text-[color:var(--text-on-yellow)] shadow-hard'
                          : 'bg-surface text-ink hover:bg-surface-alt',
                      )}
                    >
                      <span className="flex items-center gap-1.5 text-body font-extrabold">
                        <option.icon className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
                        {option.label}
                      </span>
                      <span
                        className={cn(
                          'text-[11px] leading-4',
                          selected ? 'text-[color:var(--text-on-yellow)]' : 'text-muted',
                        )}
                      >
                        {option.description}
                      </span>
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
              leadingIcon={<Mail className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
            />

            {role === 'MERCHANT' && (
              <Input
                label="Password (merchant accounts)"
                type="password"
                name="password"
                autoComplete="current-password"
                placeholder="Your signup password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                hint="Leave blank for the seeded demo accounts."
                leadingIcon={<LockKeyhole className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
              />
            )}

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
              <ArrowRight className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />
            </Button>
          </form>

          <div className="mt-6 border-t-2 border-line pt-4 text-center">
            <Link to="/signup" className="text-meta font-bold text-blue underline underline-offset-4">
              New merchant? Create an account
            </Link>
          </div>

          <div className="mt-6 border-t-2 border-line pt-4">
            <p className="text-meta font-bold text-ink">Demo credentials</p>
            <p className="mt-0.5 text-[11px] text-muted">Click one to fill the form.</p>
            <div className="mt-2.5 flex flex-col gap-2">
              {DEMO_ACCOUNTS.map((account) => (
                <button
                  key={account.email}
                  type="button"
                  onClick={() => fillDemo(account)}
                  className="inline-flex min-h-[44px] items-center gap-2 rounded border-2 border-line bg-surface px-2.5 py-1.5 text-left text-[12px] text-ink transition-colors duration-100 hover:bg-surface-alt"
                >
                  <account.icon className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2.5} />
                  <span className="font-bold">{account.label}</span>
                  <span className="ml-auto font-mono text-[11px] text-muted">{account.email}</span>
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
