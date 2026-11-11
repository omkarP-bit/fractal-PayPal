import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
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
    <div className="space-y-5">
      <Link
        to="/customer/cases"
        className="inline-flex items-center gap-1.5 text-meta font-medium text-blue hover:underline"
      >
        <ArrowLeft className="h-4 w-4" aria-hidden="true" />
        Back to my cases
      </Link>

      <PageHeader
        title="Report an issue"
        description="Tell us what went wrong with a payment. We’ll match it to your order."
      />

      <form
        onSubmit={handleSubmit}
        className="space-y-6 rounded-lg border border-line bg-surface p-6"
        noValidate
      >
        <fieldset>
          <legend className="text-body font-semibold text-ink">What went wrong?</legend>
          <p className="mt-0.5 text-meta text-muted">Choose the option that fits best.</p>
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            {ISSUE_TYPES.map((issue) => {
              const selected = form.case_type === issue.value;
              return (
                <label
                  key={issue.value}
                  className={cn(
                    'flex cursor-pointer items-start gap-2.5 rounded-md border p-3 transition-colors duration-150',
                    selected
                      ? 'border-blue bg-info-bg'
                      : 'border-line hover:border-line-strong hover:bg-surface-muted',
                  )}
                >
                  <input
                    type="radio"
                    name="case_type"
                    value={issue.value}
                    checked={selected}
                    onChange={() => update('case_type', issue.value)}
                    className="mt-0.5 h-4 w-4 shrink-0 accent-blue"
                  />
                  <span className="min-w-0">
                    <span className="block text-body font-medium text-ink">{issue.label}</span>
                    <span className="mt-0.5 block text-[12px] leading-4 text-muted">
                      {issue.description}
                    </span>
                  </span>
                </label>
              );
            })}
          </div>
        </fieldset>

        <div className="space-y-4 border-t border-line pt-6">
          <div>
            <h2 className="text-body font-semibold text-ink">Your order</h2>
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

        <div className="space-y-4 border-t border-line pt-6">
          <div>
            <h2 className="text-body font-semibold text-ink">Prove this is your order</h2>
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

        <div className="border-t border-line pt-6">
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
            <span className="shrink-0 tabular-nums">
              {form.description.length}/{MAX_DESCRIPTION}
            </span>
          </div>

          {/* TODO(backend): add an evidence-upload endpoint, then enable a drag-and-drop
              uploader here. Until then evidence can be added from the case conversation. */}
          <p className="mt-3 rounded-md bg-surface-muted px-3 py-2 text-[12px] leading-5 text-muted">
            You can add evidence to the case after you submit it.
          </p>
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
    </div>
  );
}
