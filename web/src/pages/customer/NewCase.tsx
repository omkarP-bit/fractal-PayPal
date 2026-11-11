import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowLeft, Check, FileText, Upload } from 'lucide-react';
import { toast } from 'sonner';
import { apiClient } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { cn } from '../../lib/cn';
import { apiErrorMessage } from '../../lib/format';
import { Button } from '../../components/ui/Button';
import { Input, Textarea } from '../../components/ui/Input';
import { ErrorNotice } from '../../components/ui/States';
import { PageHeader } from '../../components/ui/Panel';

type CaseType = 'PROVIDER_MERCHANT_STATE_MISMATCH' | 'AMOUNT_MISMATCH' | 'DUPLICATE_CHARGE' | 'OTHER';

const ISSUE_TYPES: Array<{ value: CaseType; label: string; description: string }> = [
  {
    value: 'PROVIDER_MERCHANT_STATE_MISMATCH',
    label: 'Payment and order don’t match',
    description: 'I was charged, but my order shows unpaid or a different status.',
  },
  {
    value: 'AMOUNT_MISMATCH',
    label: 'The amount is wrong',
    description: 'I was charged a different amount than my order total.',
  },
  {
    value: 'DUPLICATE_CHARGE',
    label: 'I was charged twice',
    description: 'The same payment appears more than once.',
  },
  {
    value: 'OTHER',
    label: 'Something else',
    description: 'Another payment problem I’d like help with.',
  },
];

const STEPS = [
  'We match your payment to the order you name here.',
  'Your merchant sees the case and can reconcile the order.',
  'You get every update in the conversation and the case timeline.',
];

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const MAX_DESCRIPTION = 1000;

export function NewCase() {
  const { user } = useAuthStore();
  const navigate = useNavigate();
  const [form, setForm] = useState({
    merchant_id: '',
    merchant_order_ref: '',
    email: user?.email ?? '',
    token: '',
    case_type: 'PROVIDER_MERCHANT_STATE_MISMATCH' as CaseType,
    description: '',
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const update = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) => {
    setForm((current) => ({ ...current, [key]: value }));
    if (errors[key]) {
      setErrors((current) => {
        const next = { ...current };
        delete next[key];
        return next;
      });
    }
  };

  const validate = () => {
    const next: Record<string, string> = {};
    if (!form.merchant_order_ref.trim()) next.merchant_order_ref = 'Enter your order reference.';
    if (!EMAIL_PATTERN.test(form.email.trim())) next.email = 'Enter a valid email address.';
    if (!form.token.trim()) next.token = 'Enter the claim code from your order.';
    if (!form.description.trim()) next.description = 'Tell us what happened.';
    if (form.description.length > MAX_DESCRIPTION) {
      next.description = `Keep your description under ${MAX_DESCRIPTION} characters.`;
    }
    return next;
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    setSubmitError(null);
    const nextErrors = validate();
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;

    setLoading(true);
    try {
      const created = await apiClient.createCase({
        merchant_id: form.merchant_id,
        merchant_order_ref: form.merchant_order_ref.trim(),
        claim: { email: form.email.trim(), token: form.token.trim() },
        case_type: form.case_type,
        description: form.description.trim(),
      });
      toast.success('Report submitted', {
        description: 'We’ll keep you updated as your case moves forward.',
      });
      navigate(`/customer/cases/${created.id}`);
    } catch (err) {
      setSubmitError(apiErrorMessage(err, 'We couldn’t submit your report. Try again.'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="page-fill gap-5">
      <div>
        <Link
          to="/customer/cases"
          className="inline-flex items-center gap-1.5 text-meta font-bold text-blue hover:underline"
        >
          <ArrowLeft className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />
          Back to my cases
        </Link>
        <PageHeader
          className="mt-3"
          title="Report an issue"
          description="Tell us what went wrong with a payment. We’ll match it to your order."
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,720px)_minmax(280px,1fr)] xl:items-start">
        <form onSubmit={handleSubmit} className="panel space-y-6" noValidate>
          <fieldset>
            <legend className="text-section font-bold text-ink">What went wrong?</legend>
            <p className="mt-0.5 text-meta text-muted">Choose the option that fits best.</p>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              {ISSUE_TYPES.map((issue) => {
                const selected = form.case_type === issue.value;
                return (
                  <label
                    key={issue.value}
                    className={cn(
                      'flex cursor-pointer items-start gap-2.5 rounded border-2 border-line p-3 transition-[background-color,box-shadow] duration-100',
                      selected
                        ? 'bg-yellow text-[color:var(--text-on-yellow)] shadow-hard'
                        : 'bg-surface text-ink hover:bg-surface-alt',
                    )}
                  >
                    <input
                      type="radio"
                      name="case_type"
                      value={issue.value}
                      checked={selected}
                      onChange={() => update('case_type', issue.value)}
                      className="mt-0.5 h-4 w-4 shrink-0 accent-[color:var(--brand-blue)]"
                    />
                    <span className="min-w-0">
                      <span className="block text-body font-bold">{issue.label}</span>
                      <span
                        className={cn(
                          'mt-0.5 block text-[12px] leading-4',
                          selected ? 'text-[color:var(--text-on-yellow)]' : 'text-muted',
                        )}
                      >
                        {issue.description}
                      </span>
                    </span>
                  </label>
                );
              })}
            </div>
          </fieldset>

          <div className="space-y-4 border-t-2 border-line pt-6">
            <div>
              <h2 className="text-section font-bold text-ink">Your order</h2>
              <p className="mt-0.5 text-meta text-muted">
                Use the order reference from your confirmation.
              </p>
            </div>

            {/* TODO(backend): replace this manual field with a searchable order picker
                once an orders-list endpoint is available. */}
            <Input
              label="Order reference"
              value={form.merchant_order_ref}
              onChange={(event) => update('merchant_order_ref', event.target.value)}
              placeholder="ORDER-1234"
              error={errors.merchant_order_ref}
              autoComplete="off"
              required
            />

            <Input
              label="Store ID (optional)"
              value={form.merchant_id}
              onChange={(event) => update('merchant_id', event.target.value)}
              placeholder="Usually not needed"
              hint="We find the store from your claim code."
              autoComplete="off"
            />
          </div>

          <div className="space-y-4 border-t-2 border-line pt-6">
            <div>
              <h2 className="text-section font-bold text-ink">Prove this is your order</h2>
              <p className="mt-0.5 text-meta text-muted">
                We use the email and claim code from your confirmation page.
              </p>
            </div>

            <Input
              label="Email address"
              type="email"
              value={form.email}
              onChange={(event) => update('email', event.target.value)}
              placeholder="buyer@example.com"
              error={errors.email}
              autoComplete="email"
              inputMode="email"
              required
            />

            <Input
              label="Claim code"
              value={form.token}
              onChange={(event) => update('token', event.target.value)}
              placeholder="K7Q2-M9XD"
              error={errors.token}
              hint="Found on your order confirmation page."
              autoComplete="off"
              required
            />
          </div>

          <div className="border-t-2 border-line pt-6">
            <Textarea
              label="What happened?"
              value={form.description}
              onChange={(event) => update('description', event.target.value.slice(0, MAX_DESCRIPTION))}
              placeholder="I was charged but the store says unpaid…"
              error={errors.description}
              rows={4}
              maxLength={MAX_DESCRIPTION}
              required
            />
            <div className="mt-1 flex items-center justify-between gap-3 text-[11px] text-muted">
              <span>Include what you expected and what you saw.</span>
              <span className="shrink-0 font-mono tabular-nums">
                {form.description.length}/{MAX_DESCRIPTION}
              </span>
            </div>

            {/* TODO(backend): add an evidence-upload endpoint, then enable a drag-and-drop
                uploader here. Until then evidence can be added from the case conversation. */}
            <div className="mt-4 flex flex-col items-center gap-2 rounded border-2 border-dashed border-line bg-surface-alt px-4 py-6 text-center">
              <Upload className="h-6 w-6 text-ink" aria-hidden="true" strokeWidth={2.5} />
              <p className="text-body font-bold text-ink">Evidence upload</p>
              <p className="max-w-sm text-meta text-muted">
                You can add evidence to the case after you submit it.
              </p>
            </div>
          </div>

          {submitError && (
            <ErrorNotice title="We couldn’t submit your report" description={submitError} />
          )}

          <Button
            type="submit"
            variant="primary"
            size="lg"
            className="w-full"
            isLoading={loading}
            loadingText="Submitting report…"
          >
            Submit report
          </Button>
        </form>

        <aside className="space-y-5 xl:sticky xl:top-24">
          <section className="panel p-5">
            <div className="flex items-center gap-2">
              <FileText className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2.5} />
              <h2 className="text-section font-bold text-ink">What happens next</h2>
            </div>
            <ol className="mt-4 space-y-3">
              {STEPS.map((step, index) => (
                <li key={step} className="flex items-start gap-3">
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded border-2 border-line bg-surface-alt font-mono text-[12px] font-bold text-ink">
                    {index + 1}
                  </span>
                  <p className="text-meta font-medium text-ink">{step}</p>
                </li>
              ))}
            </ol>
          </section>

          <section className="panel p-5">
            <h2 className="text-body font-bold text-ink">Before you submit</h2>
            <ul className="mt-3 space-y-2">
              {[
                'The order reference matches your confirmation email.',
                'The email is the one you used to pay.',
                'Your description says what you expected and what you saw.',
              ].map((item) => (
                <li key={item} className="flex items-start gap-2 text-meta text-muted">
                  <Check className="mt-0.5 h-3.5 w-3.5 shrink-0 text-ink" aria-hidden="true" strokeWidth={3} />
                  {item}
                </li>
              ))}
            </ul>
          </section>
        </aside>
      </div>
    </div>
  );
}
