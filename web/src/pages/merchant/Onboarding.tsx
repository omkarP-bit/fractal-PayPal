import { useEffect, useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  ArrowRight,
  BadgeCheck,
  CheckCircle2,
  Globe2,
  KeyRound,
  PlugZap,
  RefreshCw,
  ShieldCheck,
  Store,
} from 'lucide-react';
import { toast } from 'sonner';
import { apiClient, type OnboardingStatus, type ProviderAccountSummary, type Region } from '../../services/api';
import { useAuthStore, type User } from '../../store/auth';
import { apiErrorMessage, formatAbsolute } from '../../lib/format';
import { cn } from '../../lib/cn';
import { Button } from '../../components/ui/Button';
import { Input, Select } from '../../components/ui/Input';
import { PageHeader } from '../../components/ui/Panel';
import { StatusPill } from '../../components/ui/StatusPill';
import { CopyChip } from '../../components/ui/CopyChip';
import { ErrorNotice, Skeleton } from '../../components/ui/States';
import { Stepper, type Step } from '../../components/ui/Stepper';

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

type SignupFields = {
  email: string;
  password: string;
  displayName: string;
  legalName: string;
  country: string;
  currency: string;
};

const EMPTY_FIELDS: SignupFields = {
  email: '',
  password: '',
  displayName: '',
  legalName: '',
  country: '',
  currency: '',
};

function regionLabel(region: Region): string {
  return `${region.name} (${region.code}) · ${region.currency}`;
}

function webhookTone(status: string): 'success' | 'warning' | 'danger' | 'neutral' {
  switch (status.toUpperCase()) {
    case 'REGISTERED':
      return 'success';
    case 'REGISTERING':
    case 'PENDING':
      return 'warning';
    case 'FAILED':
      return 'danger';
    default:
      return 'neutral';
  }
}

function webhookLabel(status: string): string {
  switch (status.toUpperCase()) {
    case 'REGISTERED':
      return 'Webhook live';
    case 'REGISTERING':
      return 'Registering';
    case 'PENDING':
      return 'Not registered';
    case 'FAILED':
      return 'Registration failed';
    case 'DISABLED':
      return 'Disabled';
    default:
      return status;
  }
}

/** Shared Connect PayPal form. The secret stays in component state only. */
function PayPalForm({
  onConnected,
  compact = false,
}: {
  onConnected: (account: ProviderAccountSummary) => void;
  compact?: boolean;
}) {
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [environment, setEnvironment] = useState('SANDBOX');
  const [touched, setTouched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const clientIdError = touched && !clientId.trim() ? 'Paste the Client ID from your PayPal app.' : '';
  const secretError = touched && !clientSecret.trim() ? 'Paste the Secret from your PayPal app.' : '';

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setTouched(true);
    setError('');
    if (!clientId.trim() || !clientSecret.trim()) return;

    setLoading(true);
    try {
      const result = await apiClient.connectPayPal({
        client_id: clientId.trim(),
        client_secret: clientSecret.trim(),
        environment,
      });
      setClientId('');
      setClientSecret('');
      onConnected(result.provider_account);
      toast.success('PayPal connected', {
        description: 'The merchant webhook is registered and ready to receive events.',
      });
    } catch (err) {
      setError(apiErrorMessage(err, 'We could not connect this PayPal app. Check the credentials and try again.'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <form onSubmit={submit} className={cn('space-y-4', compact ? '' : 'p-1')} noValidate>
      <div className="grid gap-4 sm:grid-cols-2">
        <Input
          label="PayPal Client ID"
          name="paypal_client_id"
          autoComplete="off"
          spellCheck={false}
          value={clientId}
          onChange={(event) => setClientId(event.target.value)}
          onBlur={() => setTouched(true)}
          error={clientIdError}
          required
          hint="PayPal Developer Dashboard → Apps & Credentials → Sandbox → your app."
          leadingIcon={<KeyRound className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
        />
        <Input
          label="PayPal Secret"
          name="paypal_client_secret"
          type="password"
          autoComplete="new-password"
          spellCheck={false}
          value={clientSecret}
          onChange={(event) => setClientSecret(event.target.value)}
          onBlur={() => setTouched(true)}
          error={secretError}
          required
          hint="Encrypted before it is stored. It is never shown again."
          leadingIcon={<ShieldCheck className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
        />
      </div>

      <Select
        label="PayPal environment"
        value={environment}
        onChange={(event) => setEnvironment(event.target.value)}
        options={[
          { value: 'SANDBOX', label: 'Sandbox' },
          { value: 'LIVE', label: 'Live (not enabled in this build)', disabled: true },
        ]}
        hint="Registering the webhook happens automatically for this merchant."
      />

      <p className="rounded border-2 border-line bg-surface-alt p-3 text-meta leading-5 text-muted">
        FRACTAL does not create PayPal apps. Create a sandbox REST app in the{' '}
        <a
          href="https://developer.paypal.com/dashboard/applications/sandbox"
          target="_blank"
          rel="noreferrer"
          className="font-bold text-blue underline underline-offset-4"
        >
          PayPal Developer Dashboard
        </a>{' '}
        and paste its Client ID and Secret. Webhook registration needs a public HTTPS URL, so open FRACTAL through its
        tunnel or set <code className="font-mono text-[11px]">FRACTAL_PUBLIC_URL</code> to the public origin.
      </p>

      {error && <ErrorNotice title="PayPal connection failed" description={error} />}

      <Button
        type="submit"
        variant="primary"
        size="lg"
        className="w-full sm:w-auto"
        isLoading={loading}
        loadingText="Registering webhook…"
        icon={<PlugZap className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />}
      >
        Connect PayPal
      </Button>
    </form>
  );
}

function AccountPanel({
  merchant,
  region,
}: {
  merchant: OnboardingStatus['merchant'];
  region?: Region;
}) {
  return (
    <section className="panel-flush min-w-0 overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b-2 border-line bg-surface-alt px-4 py-3">
        <div className="min-w-0">
          <h2 className="text-[12px] font-bold uppercase tracking-[0.04em] text-ink">Business &amp; region</h2>
          <p className="mt-0.5 text-meta text-muted">Region sets the settlement currency for this merchant.</p>
        </div>
        <StatusPill
          meta={{
            label: merchant.onboarding_status === 'ACTIVE' ? 'Onboarding complete' : 'PayPal pending',
            tone: merchant.onboarding_status === 'ACTIVE' ? 'success' : 'warning',
          }}
        />
      </div>
      <dl className="grid gap-0 sm:grid-cols-2">
        <div className="border-b-2 border-line px-4 py-3 sm:border-b-0 sm:border-r-2">
          <dt className="meta-label">Business</dt>
          <dd className="mt-0.5 text-body font-bold text-ink">{merchant.legal_name}</dd>
        </div>
        <div className="border-b-2 border-line px-4 py-3 sm:border-b-0">
          <dt className="meta-label">Region</dt>
          <dd className="mt-0.5 text-body font-bold text-ink">
            {region ? `${region.name} (${region.code})` : merchant.country}
          </dd>
        </div>
        <div className="border-b-2 border-line px-4 py-3 sm:border-b-0 sm:border-r-2 sm:border-t-2">
          <dt className="meta-label">Settlement currency</dt>
          <dd className="mt-0.5 flex items-center gap-2 text-body font-bold text-ink">
            <Globe2 className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
            {merchant.currency}
          </dd>
        </div>
        <div className="px-4 py-3 sm:border-t-2">
          <dt className="meta-label">Provider account</dt>
          <dd className="mt-0.5 text-body font-bold text-ink">PayPal</dd>
        </div>
      </dl>
      {region?.cross_border_only && (
        <p className="border-t-2 border-line bg-warning px-4 py-3 text-meta font-medium text-[color:var(--warning-ink)]">
          {region.note}
        </p>
      )}
    </section>
  );
}

function ConnectedPanel({
  account,
  onReRegister,
  reRegistering,
}: {
  account: ProviderAccountSummary;
  onReRegister: () => void;
  reRegistering: boolean;
}) {
  return (
    <section className="panel-flush min-w-0 overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b-2 border-line bg-surface-alt px-4 py-3">
        <div className="min-w-0">
          <h2 className="text-[12px] font-bold uppercase tracking-[0.04em] text-ink">Webhook status</h2>
          <p className="mt-0.5 text-meta text-muted">PayPal signs every delivery with this merchant&apos;s app.</p>
        </div>
        <StatusPill
          meta={{ label: webhookLabel(account.webhook_status), tone: webhookTone(account.webhook_status) }}
        />
      </div>
      <dl className="divide-y-2 divide-line">
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3">
          <dt className="meta-label">Webhook URL</dt>
          <dd className="flex min-w-0 items-center gap-2">
            {account.webhook_url ? (
              <>
                <span className="truncate font-mono text-[12px] text-ink">{account.webhook_url}</span>
                <CopyChip value={account.webhook_url} label="Copy" title="Copy webhook URL" />
              </>
            ) : (
              <span className="text-meta text-muted">Not registered</span>
            )}
          </dd>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3">
          <dt className="meta-label">PayPal webhook ID</dt>
          <dd>
            {account.webhook_id ? (
              <CopyChip value={account.webhook_id} label={account.webhook_id} title="Copy webhook ID" />
            ) : (
              <span className="text-meta text-muted">Not registered</span>
            )}
          </dd>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3">
          <dt className="meta-label">Registered</dt>
          <dd className="text-meta font-medium text-ink">
            {account.webhook_registered_at ? formatAbsolute(account.webhook_registered_at) : '—'}
          </dd>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 py-3">
          <dt className="meta-label">Environment</dt>
          <dd className="text-meta font-bold uppercase text-ink">{account.environment}</dd>
        </div>
      </dl>
      {account.webhook_error && (
        <div className="border-t-2 border-line p-4">
          <ErrorNotice title="Last registration attempt failed" description={account.webhook_error} />
        </div>
      )}
      <div className="flex flex-wrap items-center gap-2 border-t-2 border-line bg-surface-alt px-4 py-3">
        <Button
          variant="outline"
          size="sm"
          onClick={onReRegister}
          isLoading={reRegistering}
          loadingText="Re-registering…"
          icon={<RefreshCw className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
        >
          Re-register webhook
        </Button>
        <span className="text-meta text-muted">
          Use this if the webhook was deleted in PayPal or the public URL changed.
        </span>
      </div>
    </section>
  );
}

/** Public signup screen. It stays public after account creation so step 2 can
 *  complete without bouncing the merchant through the sign-in page. */
export function MerchantSignupPage() {
  const [regions, setRegions] = useState<Region[]>([]);
  const [fields, setFields] = useState<SignupFields>(EMPTY_FIELDS);
  const [touched, setTouched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [account, setAccount] = useState<{ merchant: OnboardingStatus['merchant']; connected: boolean } | null>(
    null,
  );
  const login = useAuthStore((state) => state.login);
  const user = useAuthStore((state) => state.user);
  const navigate = useNavigate();

  useEffect(() => {
    apiClient
      .getRegions()
      .then(setRegions)
      .catch(() => setError('We could not load the region list. Refresh the page and try again.'));
  }, []);

  const selectedRegion = useMemo(
    () => regions.find((region) => region.code === fields.country),
    [regions, fields.country],
  );

  const emailError = touched && !EMAIL_PATTERN.test(fields.email.trim()) ? 'Enter a valid email address.' : '';
  const passwordError = touched && fields.password.length < 8 ? 'Use at least 8 characters.' : '';
  const legalNameError = touched && !fields.legalName.trim() ? 'Enter your business name.' : '';
  const regionError = touched && !fields.country ? 'Choose your region.' : '';

  const update = (key: keyof SignupFields, value: string) => {
    setFields((current) => {
      if (key === 'country') {
        const region = regions.find((item) => item.code === value);
        return { ...current, country: value, currency: region?.currency ?? '' };
      }
      return { ...current, [key]: value };
    });
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setTouched(true);
    setError('');
    if (
      !EMAIL_PATTERN.test(fields.email.trim()) ||
      fields.password.length < 8 ||
      !fields.legalName.trim() ||
      !fields.country
    ) {
      return;
    }
    setLoading(true);
    try {
      const result = await apiClient.signup({
        email: fields.email.trim(),
        password: fields.password,
        display_name: fields.displayName.trim(),
        legal_name: fields.legalName.trim(),
        country: fields.country,
        currency: fields.currency || selectedRegion?.currency || '',
      });
      login(result.token, result.user as User);
      setAccount({ merchant: result.merchant, connected: false });
      toast.success('Account created', { description: 'Now connect your PayPal sandbox app.' });
    } catch (err) {
      setError(apiErrorMessage(err, 'We could not create your account. Try again.'));
    } finally {
      setLoading(false);
    }
  };

  const steps: Step[] = [
    { label: 'Business & region', state: account ? 'complete' : 'current' },
    { label: 'Connect PayPal', state: account ? (account.connected ? 'complete' : 'current') : 'upcoming' },
    { label: 'Webhook live', state: account?.connected ? 'complete' : 'upcoming' },
  ];

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-bg px-4 py-12">
      <main className="w-full max-w-[920px]">
        <div className="mb-7 flex flex-col items-center text-center">
          <span className="mb-4 flex h-12 w-12 items-center justify-center rounded border-2 border-line bg-[#003087] text-[22px] font-extrabold text-white shadow-hard">
            F
          </span>
          <h1 className="text-title font-extrabold text-ink">Create your FRACTAL merchant account</h1>
          <p className="mt-1.5 max-w-xl text-body text-muted">
            Choose your region, then connect PayPal. FRACTAL registers the webhook for you.
          </p>
        </div>

        <div className="mb-6 rounded border-2 border-line bg-surface p-4 shadow-hard">
          <Stepper steps={steps} />
        </div>

        {!account ? (
          <section className="brut-panel rounded border-2 border-line bg-surface p-6 shadow-hard-lg sm:p-8">
            <form onSubmit={submit} className="space-y-5" noValidate>
              <div className="grid gap-4 sm:grid-cols-2">
                <Input
                  label="Your email"
                  type="email"
                  name="email"
                  autoComplete="email"
                  inputMode="email"
                  placeholder="you@business.com"
                  value={fields.email}
                  onChange={(event) => update('email', event.target.value)}
                  onBlur={() => setTouched(true)}
                  error={emailError}
                  required
                />
                <Input
                  label="Password"
                  type="password"
                  name="password"
                  autoComplete="new-password"
                  placeholder="At least 8 characters"
                  value={fields.password}
                  onChange={(event) => update('password', event.target.value)}
                  onBlur={() => setTouched(true)}
                  error={passwordError}
                  required
                  hint="Stored as a one-way hash."
                />
                <Input
                  label="Your name"
                  name="display_name"
                  autoComplete="name"
                  placeholder="Ada Merchant"
                  value={fields.displayName}
                  onChange={(event) => update('displayName', event.target.value)}
                  hint="Shown on cases you handle."
                />
                <Input
                  label="Legal business name"
                  name="legal_name"
                  autoComplete="organization"
                  placeholder="Acme Export Co."
                  value={fields.legalName}
                  onChange={(event) => update('legalName', event.target.value)}
                  onBlur={() => setTouched(true)}
                  error={legalNameError}
                  required
                />
              </div>

              <div className="grid gap-4 sm:grid-cols-2">
                <Select
                  label="Business region"
                  value={fields.country}
                  onChange={(event) => update('country', event.target.value)}
                  onBlur={() => setTouched(true)}
                  error={regionError}
                  required
                  options={[
                    { value: '', label: 'Choose a country' },
                    ...regions.map((region) => ({ value: region.code, label: regionLabel(region) })),
                  ]}
                  hint="Required. The region fixes the currency used in cases."
                />
                <Select
                  label="Settlement currency"
                  value={fields.currency}
                  onChange={(event) => update('currency', event.target.value)}
                  required
                  options={(selectedRegion?.currencies ?? []).map((currency) => ({
                    value: currency,
                    label: currency,
                  }))}
                  hint={
                    selectedRegion?.cross_border_only
                      ? 'Cross-border only: use a foreign settlement currency.'
                      : 'Chosen from your region.'
                  }
                  disabled={!selectedRegion}
                />
              </div>

              {selectedRegion?.cross_border_only && (
                <div className="flex items-start gap-2 rounded border-2 border-line bg-warning p-3 text-[color:var(--warning-ink)]">
                  <BadgeCheck className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" strokeWidth={2.5} />
                  <p className="text-meta font-medium">{selectedRegion.note}</p>
                </div>
              )}

              {error && <ErrorNotice title="Signup failed" description={error} />}

              <div className="flex flex-wrap items-center gap-3">
                <Button
                  type="submit"
                  variant="primary"
                  size="lg"
                  isLoading={loading}
                  loadingText="Creating account…"
                  icon={<Store className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />}
                >
                  Create account
                  <ArrowRight className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />
                </Button>
                <Link to="/login" className="text-meta font-bold text-blue underline underline-offset-4">
                  Already have an account?
                </Link>
              </div>
            </form>
          </section>
        ) : (
          <div className="space-y-5">
            <AccountPanel merchant={account.merchant} region={regions.find((r) => r.code === account.merchant.country)} />
            {account.connected ? (
              <section className="brut-panel flex flex-col items-start gap-4 rounded border-2 border-line bg-surface p-6 shadow-hard-lg sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-start gap-3">
                  <CheckCircle2 className="mt-0.5 h-6 w-6 text-[color:var(--success-ink)]" aria-hidden="true" strokeWidth={2.5} />
                  <div>
                    <h2 className="text-section font-bold text-ink">You are ready to receive webhooks</h2>
                    <p className="mt-0.5 text-body text-muted">
                      PayPal will deliver events to this merchant&apos;s verified endpoint.
                    </p>
                  </div>
                </div>
                <Button variant="primary" onClick={() => navigate('/merchant/cases')}>
                  Open merchant cases
                  <ArrowRight className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />
                </Button>
              </section>
            ) : (
              <section className="brut-panel rounded border-2 border-line bg-surface p-6 shadow-hard-lg">
                <div className="mb-4 flex items-start gap-3 border-b-2 border-line pb-4">
                  <PlugZap className="mt-0.5 h-5 w-5 text-ink" aria-hidden="true" strokeWidth={2.5} />
                  <div>
                    <h2 className="text-section font-bold text-ink">Connect your PayPal sandbox app</h2>
                    <p className="mt-0.5 text-body text-muted">
                      FRACTAL gets a token, registers a webhook for this merchant, and stores the returned webhook ID.
                    </p>
                  </div>
                </div>
                <PayPalForm
                  onConnected={() => {
                    setAccount((current) => (current ? { ...current, connected: true } : current));
                  }}
                />
              </section>
            )}
          </div>
        )}

        {user && (
          <p className="mt-5 text-center text-[11px] text-muted">
            Signed in as {user.email}. <Link to="/merchant/onboarding" className="font-bold text-blue underline underline-offset-4">Open PayPal setup</Link>
          </p>
        )}
      </main>
    </div>
  );
}

/** Authenticated merchant setup/reconnect screen. */
export function MerchantOnboardingPage() {
  const [status, setStatus] = useState<OnboardingStatus | null>(null);
  const [regions, setRegions] = useState<Region[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [reRegistering, setReRegistering] = useState(false);
  const navigate = useNavigate();

  const load = async () => {
    setLoading(true);
    setError('');
    try {
      const [next, regionList] = await Promise.all([apiClient.getOnboardingStatus(), apiClient.getRegions()]);
      setStatus(next);
      setRegions(regionList);
    } catch (err) {
      setError(apiErrorMessage(err, 'We could not load your PayPal setup.'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const reRegister = async () => {
    setReRegistering(true);
    setError('');
    try {
      const result = await apiClient.reRegisterWebhook();
      setStatus((current) =>
        current
          ? {
              ...current,
              provider_account: result.provider_account,
              ready: result.provider_account.webhook_status === 'REGISTERED',
              next_step: result.provider_account.webhook_status === 'REGISTERED' ? 'done' : 'retry_webhook',
            }
          : current,
      );
      toast.success('Webhook re-registered', { description: 'PayPal will now deliver events to the new registration.' });
    } catch (err) {
      const message = apiErrorMessage(err, 'We could not re-register the webhook.');
      try {
        const next = await apiClient.getOnboardingStatus();
        setStatus(next);
      } catch {
        // Keep the previous status if the refresh also fails.
      }
      setError(message);
    } finally {
      setReRegistering(false);
    }
  };

  const region = useMemo(
    () => regions.find((item) => item.code === status?.merchant.country),
    [regions, status?.merchant.country],
  );

  return (
    <div className="page-fill gap-5">
      <PageHeader
        title="PayPal setup"
        description="Connect this merchant's PayPal sandbox app. Region and currency are locked when the account is created."
        actions={
          status && (
            <StatusPill
              meta={{
                label: status.ready ? 'Ready for webhooks' : 'Setup incomplete',
                tone: status.ready ? 'success' : 'warning',
              }}
            />
          )
        }
      />

      {error && status?.ready && (
        <ErrorNotice title="Webhook action failed" description={error} onRetry={() => void load()} />
      )}

      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-40 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      ) : error && !status ? (
        <ErrorNotice title="Could not load PayPal setup" description={error} onRetry={() => void load()} />
      ) : status ? (
        <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
          <div className="min-w-0 space-y-5">
            <AccountPanel merchant={status.merchant} region={region} />

            {status.provider_account ? (
              <ConnectedPanel
                account={status.provider_account}
                onReRegister={() => void reRegister()}
                reRegistering={reRegistering}
              />
            ) : null}

            {!status.ready ? (
              <section className="brut-panel rounded border-2 border-line bg-surface p-5 shadow-hard-lg sm:p-6">
                <div className="mb-4 flex items-start gap-3 border-b-2 border-line pb-4">
                  <PlugZap className="mt-0.5 h-5 w-5 text-ink" aria-hidden="true" strokeWidth={2.5} />
                  <div>
                    <h2 className="text-section font-bold text-ink">
                      {status.provider_account ? 'Update PayPal credentials' : 'Connect PayPal'}
                    </h2>
                    <p className="mt-0.5 text-body text-muted">
                      Submitting again keeps the same provider account ID, so the webhook URL stays stable.
                    </p>
                  </div>
                </div>
                {error && <ErrorNotice title="PayPal setup failed" description={error} className="mb-4" />}
                <PayPalForm
                  onConnected={(account) => {
                    setStatus((current) =>
                      current
                        ? {
                            ...current,
                            provider_account: account,
                            ready: account.webhook_status === 'REGISTERED',
                            next_step: account.webhook_status === 'REGISTERED' ? 'done' : 'retry_webhook',
                          }
                        : current,
                    );
                  }}
                />
              </section>
            ) : (
              <section className="panel flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-start gap-3">
                  <CheckCircle2 className="mt-0.5 h-5 w-5 text-[color:var(--success-ink)]" aria-hidden="true" strokeWidth={2.5} />
                  <div>
                    <h2 className="text-body font-extrabold text-ink">Webhook live</h2>
                    <p className="mt-0.5 text-meta text-muted">
                      Every event is verified with this merchant&apos;s webhook ID before it is stored.
                    </p>
                  </div>
                </div>
                <Button variant="primary" onClick={() => navigate('/merchant/cases')}>
                  Go to cases
                  <ArrowRight className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />
                </Button>
              </section>
            )}
          </div>

          <aside className="min-w-0 space-y-5">
            <section className="panel-flush overflow-hidden">
              <div className="border-b-2 border-line bg-surface-alt px-4 py-3">
                <h2 className="text-[12px] font-bold uppercase tracking-[0.04em] text-ink">What happens next</h2>
              </div>
              <ol className="divide-y-2 divide-line">
                {[
                  ['Encrypted storage', 'The PayPal secret is sealed with the application key before it reaches PostgreSQL.'],
                  ['Automatic registration', 'FRACTAL calls PayPal and stores the returned webhook ID on this merchant.'],
                  ['Per-merchant verification', 'The receiver resolves this merchant from the URL and verifies each signature with its own app.'],
                  ['Merchant-scoped events', 'Every stored event carries this provider account ID, so cases stay scoped to this store.'],
                ].map(([title, description], index) => (
                  <li key={title} className="flex gap-3 px-4 py-3">
                    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded border-2 border-line bg-yellow text-[11px] font-extrabold text-[color:var(--text-on-yellow)]">
                      {index + 1}
                    </span>
                    <div className="min-w-0">
                      <p className="text-meta font-bold text-ink">{title}</p>
                      <p className="mt-0.5 text-[12px] leading-5 text-muted">{description}</p>
                    </div>
                  </li>
                ))}
              </ol>
            </section>

            <section className="panel-inset p-4">
              <p className="text-meta font-bold text-ink">Safety</p>
              <p className="mt-1 text-[12px] leading-5 text-muted">
                The stored secret is never returned to the browser or written to logs. Requests to the AI investigator never
                include PayPal credentials.
              </p>
            </section>
          </aside>
        </div>
      ) : null}
    </div>
  );
}
