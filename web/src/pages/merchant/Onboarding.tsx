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
  Wand2,
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
        This advanced path uses a PayPal app you own. Create a sandbox REST app in the{' '}
        <a
          href="https://developer.paypal.com/dashboard/applications/sandbox"
          target="_blank"
          rel="noreferrer"
          className="font-bold text-blue underline underline-offset-4"
        >
          PayPal Developer Dashboard
        </a>{' '}
        and paste its Client ID and Secret. Webhook registration needs the deployment&apos;s public HTTPS origin; FRACTAL
        configures that once for managed setup, so you do not run a tunnel.
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
  onUpdateCredentials,
}: {
  account: ProviderAccountSummary;
  onReRegister: () => void;
  reRegistering: boolean;
  onUpdateCredentials: () => void;
}) {
  return (
    <section className="panel-flush min-w-0 overflow-hidden">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b-2 border-line bg-surface-alt px-4 py-3">
        <div className="min-w-0">
          <h2 className="text-[12px] font-bold uppercase tracking-[0.04em] text-ink">Webhook status</h2>
          <p className="mt-0.5 text-meta text-muted">
            {account.managed
              ? "FRACTAL registers and signs this merchant's webhook with its own sandbox app."
              : "PayPal signs every delivery with this merchant's app."}
          </p>
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
          <dt className="meta-label">PayPal app</dt>
          <dd className="flex min-w-0 items-center gap-2">
            {account.managed ? (
              <span className="rounded border-2 border-line bg-yellow px-2 py-0.5 text-[11px] font-extrabold uppercase tracking-[0.04em] text-[color:var(--text-on-yellow)]">
                Managed by FRACTAL
              </span>
            ) : account.client_id ? (
              <>
                <span className="truncate font-mono text-[12px] text-ink">{account.client_id}</span>
                <CopyChip value={account.client_id} label="Copy" title="Copy Client ID" />
              </>
            ) : (
              <span className="text-meta text-muted">Using deployment credentials</span>
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
        <Button variant="outline" size="sm" onClick={onUpdateCredentials}>
          {account.managed ? 'Use my own app' : 'Update credentials'}
        </Button>
      </div>
      <p className="border-t-2 border-line px-4 py-3 text-[11px] leading-5 text-muted">
        {account.managed ? (
          <>
            This connection is managed by FRACTAL. No PayPal secret is stored on this merchant. To use your own PayPal
            app instead, choose <span className="font-bold text-ink">Use my own app</span>.
          </>
        ) : (
          <>
            The Client ID is shown above. The Secret is stored encrypted and can never be displayed. To rotate it,
            generate a new Secret in the PayPal app and choose{' '}
            <span className="font-bold text-ink">Update credentials</span>.
          </>
        )}
      </p>
    </section>
  );
}

/** One-click path: FRACTAL registers a webhook with its own sandbox app. */
function ManagedConnectPanel({
  onConnect,
  connecting,
  hasAccount,
}: {
  onConnect: () => void;
  connecting: boolean;
  hasAccount: boolean;
}) {
  return (
    <div className="space-y-4">
      <div className="rounded border-2 border-line bg-surface-alt p-4">
        <div className="flex items-start gap-3">
          <Wand2 className="mt-0.5 h-5 w-5 shrink-0 text-ink" aria-hidden="true" strokeWidth={2.5} />
          <div className="min-w-0">
            <h3 className="text-body font-extrabold text-ink">One-click sandbox setup</h3>
            <p className="mt-1 text-meta leading-5 text-muted">
              Use FRACTAL&apos;s PayPal sandbox app. You do not need a PayPal developer account, a REST app, credentials,
              or a tunnel. FRACTAL registers a dedicated webhook for this merchant automatically.
            </p>
          </div>
        </div>
        <ul className="mt-3 grid gap-2 sm:grid-cols-3">
          {['No app to create', 'Nothing to copy', 'No tunnel to run'].map((item) => (
            <li
              key={item}
              className="flex items-center gap-2 rounded border-2 border-line bg-surface px-3 py-2 text-[11px] font-bold text-ink"
            >
              <CheckCircle2
                className="h-4 w-4 shrink-0 text-[color:var(--success-ink)]"
                aria-hidden="true"
                strokeWidth={2.5}
              />
              {item}
            </li>
          ))}
        </ul>
      </div>

      <p className="text-[11px] leading-5 text-muted">
        PayPal sandbox apps allow 10 webhooks. That is enough for this testing cohort; if the limit is reached, FRACTAL
        tells you which registration to remove before retrying.
      </p>

      <Button
        type="button"
        variant="primary"
        size="lg"
        className="w-full sm:w-auto"
        onClick={onConnect}
        isLoading={connecting}
        loadingText="Registering webhook…"
        icon={<Wand2 className="h-5 w-5" aria-hidden="true" strokeWidth={2.5} />}
      >
        {hasAccount ? 'Retry with FRACTAL sandbox app' : 'Connect with FRACTAL sandbox app'}
      </Button>
    </div>
  );
}

/** Public signup screen. It stays public after account creation so step 2 can
 *  complete without bouncing the merchant through the sign-in page. */
export function MerchantSignupPage() {
  const [regions, setRegions] = useState<Region[]>([]);
  const [fields, setFields] = useState<SignupFields>(EMPTY_FIELDS);
  const [touched, setTouched] = useState(false);
  const [loading, setLoading] = useState(false);
  const [managedConnecting, setManagedConnecting] = useState(false);
  const [showManual, setShowManual] = useState(false);
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
      toast.success('Account created', { description: 'Now connect PayPal with one click.' });
    } catch (err) {
      setError(apiErrorMessage(err, 'We could not create your account. Try again.'));
    } finally {
      setLoading(false);
    }
  };

  const connectManaged = async () => {
    setManagedConnecting(true);
    setError('');
    try {
      await apiClient.connectManagedPayPal();
      setAccount((current) =>
        current
          ? {
              ...current,
              connected: true,
              merchant: { ...current.merchant, onboarding_status: 'ACTIVE' },
            }
          : current,
      );
      setShowManual(false);
      toast.success('PayPal connected', {
        description: 'FRACTAL registered a dedicated webhook for this merchant.',
      });
    } catch (err) {
      setError(apiErrorMessage(err, 'We could not register the managed PayPal webhook.'));
    } finally {
      setManagedConnecting(false);
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
                    <h2 className="text-section font-bold text-ink">Connect PayPal</h2>
                    <p className="mt-0.5 text-body text-muted">
                      One click uses FRACTAL&apos;s sandbox app. No PayPal app, credentials, or tunnel needed.
                    </p>
                  </div>
                </div>
                {error && <ErrorNotice title="PayPal setup failed" description={error} className="mb-4" />}
                <div className="space-y-4">
                  <ManagedConnectPanel
                    onConnect={() => void connectManaged()}
                    connecting={managedConnecting}
                    hasAccount={false}
                  />

                  <div className="flex items-center gap-3">
                    <span className="h-0.5 flex-1 bg-line" aria-hidden="true" />
                    <span className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted">Advanced</span>
                    <span className="h-0.5 flex-1 bg-line" aria-hidden="true" />
                  </div>

                  {showManual ? (
                    <div id="signup-paypal-manual-form" className="space-y-4">
                      <PayPalForm
                        onConnected={() => {
                          setError('');
                          setAccount((current) =>
                            current
                              ? {
                                  ...current,
                                  connected: true,
                                  merchant: { ...current.merchant, onboarding_status: 'ACTIVE' },
                                }
                              : current,
                          );
                          setShowManual(false);
                        }}
                      />
                      <Button type="button" variant="tertiary" size="sm" onClick={() => setShowManual(false)}>
                        Use FRACTAL&apos;s sandbox app instead
                      </Button>
                    </div>
                  ) : (
                    <Button
                      type="button"
                      variant="tertiary"
                      size="sm"
                      aria-expanded={showManual}
                      aria-controls={showManual ? 'signup-paypal-manual-form' : undefined}
                      onClick={() => setShowManual(true)}
                    >
                      I have my own PayPal app
                    </Button>
                  )}
                </div>
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
  const [managedConnecting, setManagedConnecting] = useState(false);
  const [showManual, setShowManual] = useState(false);
  const [showCredentials, setShowCredentials] = useState(false);
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

  const connectManaged = async () => {
    setManagedConnecting(true);
    setError('');
    try {
      const result = await apiClient.connectManagedPayPal();
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
      setShowManual(false);
      setShowCredentials(false);
      toast.success('PayPal connected', {
        description: 'FRACTAL registered a dedicated webhook with its sandbox app.',
      });
    } catch (err) {
      setError(apiErrorMessage(err, 'We could not register the managed PayPal webhook.'));
    } finally {
      setManagedConnecting(false);
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
        description="Connect this merchant to PayPal. One click uses FRACTAL's sandbox app; advanced users can bring their own. Region and currency are locked when the account is created."
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
                onUpdateCredentials={() => setShowCredentials(true)}
              />
            ) : null}

            {!status.ready || showCredentials ? (
              <section className="brut-panel rounded border-2 border-line bg-surface p-5 shadow-hard-lg sm:p-6">
                <div className="mb-4 flex items-start gap-3 border-b-2 border-line pb-4">
                  <PlugZap className="mt-0.5 h-5 w-5 text-ink" aria-hidden="true" strokeWidth={2.5} />
                  <div className="min-w-0">
                    <h2 className="text-section font-bold text-ink">
                      {showCredentials
                        ? 'Update PayPal credentials'
                        : status.provider_account
                          ? 'Reconnect PayPal'
                          : 'Connect PayPal'}
                    </h2>
                    <p className="mt-0.5 text-body text-muted">
                      {showCredentials
                        ? 'Submitting again keeps the same provider account ID, so the webhook URL stays stable.'
                        : "One click uses FRACTAL's sandbox app. Bring your own app under Advanced if you prefer."}
                    </p>
                  </div>
                  {status.ready && showCredentials && (
                    <Button variant="ghost" size="sm" className="ml-auto shrink-0" onClick={() => setShowCredentials(false)}>
                      Cancel
                    </Button>
                  )}
                </div>
                {error && <ErrorNotice title="PayPal setup failed" description={error} className="mb-4" />}
                {showCredentials ? (
                  <PayPalForm
                    onConnected={(account) => {
                      setError('');
                      setShowCredentials(false);
                      setShowManual(false);
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
                ) : (
                  <div className="space-y-4">
                    <ManagedConnectPanel
                      onConnect={() => void connectManaged()}
                      connecting={managedConnecting}
                      hasAccount={Boolean(status.provider_account)}
                    />

                    <div className="flex items-center gap-3">
                      <span className="h-0.5 flex-1 bg-line" aria-hidden="true" />
                      <span className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted">Advanced</span>
                      <span className="h-0.5 flex-1 bg-line" aria-hidden="true" />
                    </div>

                    {showManual ? (
                      <div id="paypal-manual-form" className="space-y-4">
                        <PayPalForm
                          onConnected={(account) => {
                            setError('');
                            setShowManual(false);
                            setShowCredentials(false);
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
                        <Button type="button" variant="tertiary" size="sm" onClick={() => setShowManual(false)}>
                          Use FRACTAL&apos;s sandbox app instead
                        </Button>
                      </div>
                    ) : (
                      <Button
                        type="button"
                        variant="tertiary"
                        size="sm"
                        aria-expanded={showManual}
                        aria-controls={showManual ? 'paypal-manual-form' : undefined}
                        onClick={() => setShowManual(true)}
                      >
                        I have my own PayPal app
                      </Button>
                    )}
                  </div>
                )}
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
                  ['No app needed by default', "The recommended path uses FRACTAL's sandbox app and registers a dedicated webhook for this merchant."],
                  ['Automatic registration', 'FRACTAL calls PayPal and stores the returned webhook ID on this merchant.'],
                  ['Per-merchant verification', 'The receiver resolves this merchant from the URL and verifies each signature with the configured app.'],
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
                PayPal credentials are never returned to the browser or written to logs. Managed accounts keep no
                merchant secret; manual accounts store only an encrypted secret. Requests to the AI investigator never
                include PayPal credentials.
              </p>
            </section>
          </aside>
        </div>
      ) : null}
    </div>
  );
}
