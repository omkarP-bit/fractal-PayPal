import { useEffect, useState, type ReactNode } from 'react';
import {
  Activity,
  CheckCircle2,
  ExternalLink,
  FlaskConical,
  Radio,
  RefreshCw,
  RotateCcw,
  Server,
  AlertTriangle,
} from 'lucide-react';
import { toast } from 'sonner';
import { cn } from '../../lib/cn';
import { Button } from '../../components/ui/Button';
import { ConfirmDialog } from '../../components/ui/Dialog';
import { PageHeader } from '../../components/ui/Panel';
import { StatusPill } from '../../components/ui/StatusPill';
import { ErrorNotice, Skeleton } from '../../components/ui/States';
import { apiClient, type ObservabilitySummary } from '../../services/api';
import { apiErrorMessage, formatAbsolute, sentenceCase } from '../../lib/format';

interface DemoAction {
  title: string;
  description: string;
  confirmLabel: string;
  destructive?: boolean;
  run: () => Promise<void> | void;
}

function delay(ms: number) {
  return new Promise<void>((resolve) => {
    window.setTimeout(resolve, ms);
  });
}

/** Grouped panel with a flat title strip, matching the detail-page rails. */
function Group({
  title,
  description,
  aside,
  children,
  className,
  bodyClassName,
  tone = 'default',
}: {
  title: string;
  description?: string;
  aside?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
  tone?: 'default' | 'danger';
}) {
  return (
    <section
      className={cn(
        'panel-flush flex min-w-0 flex-col overflow-hidden',
        tone === 'danger' && 'border-[color:var(--danger)]',
        className,
      )}
    >
      <div
        className={cn(
          'flex flex-wrap items-center justify-between gap-2 border-b-2 border-line px-4 py-3',
          tone === 'danger'
            ? 'bg-danger text-[color:var(--danger-ink)]'
            : 'bg-surface-alt text-ink',
        )}
      >
        <div className="min-w-0">
          <h2 className="text-[12px] font-bold uppercase tracking-[0.04em]">{title}</h2>
          {description && (
            <p className="mt-0.5 text-meta font-medium opacity-80">{description}</p>
          )}
        </div>
        {aside && <div className="shrink-0">{aside}</div>}
      </div>
      <div className={cn('flex min-h-0 flex-1 flex-col p-4', bodyClassName)}>{children}</div>
    </section>
  );
}

export function DemoPanel() {
  const [handlerBroken, setHandlerBroken] = useState(false);
  const [concurrencyResult, setConcurrencyResult] = useState<string | null>(null);
  const [isRunning, setIsRunning] = useState(false);
  const [action, setAction] = useState<DemoAction | null>(null);
  const [actionBusy, setActionBusy] = useState(false);
  const [live, setLive] = useState<ObservabilitySummary | null>(null);
  const [liveError, setLiveError] = useState('');
  const [liveLoading, setLiveLoading] = useState(true);

  // Real telemetry, not a mocked dashboard. Polling keeps the panel current;
  // the heavy visualisation lives in the real Kibana instance linked below.
  useEffect(() => {
    let active = true;
    const load = async () => {
      try {
        const summary = await apiClient.getObservability();
        if (!active) return;
        setLive(summary);
        setLiveError('');
      } catch (err) {
        if (!active) return;
        setLiveError(apiErrorMessage(err, 'Live webhook telemetry is unavailable.'));
      } finally {
        if (active) setLiveLoading(false);
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 10000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);

  const runAction = async () => {
    if (!action) return;
    setActionBusy(true);
    try {
      await action.run();
    } finally {
      setActionBusy(false);
      setAction(null);
    }
  };

  const simulate = async (title: string, description: string) => {
    await delay(700);
    toast.success(title, { description });
  };

  const toggleHandler = () => {
    const next = !handlerBroken;
    setHandlerBroken(next);
    toast[next ? 'warning' : 'success'](
      next ? 'Webhook handler is broken' : 'Webhook handler restored',
      {
        description: next
          ? 'New webhook deliveries will fail until you restore it.'
          : 'New webhook deliveries will process normally.',
      },
    );
  };

  const runConcurrencyTest = async () => {
    setIsRunning(true);
    setConcurrencyResult(null);
    await delay(1800);
    setConcurrencyResult('1 executed, 49 duplicates blocked');
    setIsRunning(false);
    toast.success('Concurrency test finished', {
      description: 'One replay executed and 49 duplicate attempts were blocked.',
    });
  };

  const seedAction = (scenario: string, description: string): DemoAction => ({
    title: `Seed ${scenario.toLowerCase()}?`,
    description,
    confirmLabel: 'Seed scenario',
    run: () => simulate('Scenario seeded', `${scenario} is ready in the demo case list.`),
  });

  return (
    <div className="page-fill gap-5">
      <PageHeader
        title="Demo controls"
        description="Simulated scenario tools plus live, merchant-scoped webhook telemetry."
        actions={
          <span className="inline-flex items-center gap-1.5 rounded-pill border-2 border-line bg-warning px-2.5 py-1 text-[11px] font-bold uppercase tracking-[0.04em] text-[color:var(--warning-ink)] shadow-hard">
            <FlaskConical className="h-3.5 w-3.5" aria-hidden="true" strokeWidth={2.5} />
            Local only
          </span>
        }
      />

      <div className="grid gap-5 lg:grid-cols-3">
        <Group
          title="Webhook handling"
          description="Control how the mock store responds to payment notifications."
          className="lg:col-span-2"
          aside={
            <StatusPill
              meta={
                handlerBroken
                  ? { label: 'Handler broken', tone: 'danger' }
                  : { label: 'Handler working', tone: 'success' }
              }
            />
          }
        >
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
            <p className="flex min-w-0 flex-1 items-start gap-2 text-body text-ink">
              <Server className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" strokeWidth={2.5} />
              {handlerBroken
                ? 'Webhook deliveries are failing. Restore the handler to resume the demo flow.'
                : 'Webhook deliveries are processed normally. Break it to test failure handling.'}
            </p>
            <Button
              variant={handlerBroken ? 'secondary' : 'danger'}
              className="w-full sm:w-auto"
              onClick={() =>
                setAction({
                  title: handlerBroken ? 'Restore the webhook handler?' : 'Break the webhook handler?',
                  description: handlerBroken
                    ? 'New webhook deliveries will start processing again.'
                    : 'New webhook deliveries will fail until you restore it. Existing data is not changed.',
                  confirmLabel: handlerBroken ? 'Restore handler' : 'Break handler',
                  destructive: !handlerBroken,
                  run: toggleHandler,
                })
              }
            >
              {handlerBroken ? 'Restore handler' : 'Break handler'}
            </Button>
          </div>
        </Group>

        <Group title="Concurrency test" description="Fire 50 replays at the same time.">
          <Button
            variant="primary"
            className="w-full"
            onClick={() =>
              setAction({
                title: 'Run 50 concurrent replays?',
                description:
                  'This stress-tests idempotency. Exactly one replay should execute and the rest should be blocked.',
                confirmLabel: 'Run test',
                run: runConcurrencyTest,
              })
            }
            isLoading={isRunning}
            loadingText="Running test…"
            icon={<FlaskConical className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
          >
            Run 50 concurrent replays
          </Button>
          {concurrencyResult ? (
            <div className="mt-3 flex items-start gap-2 rounded border-2 border-line bg-success p-3 text-[color:var(--success-ink)]">
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" strokeWidth={2.5} />
              <div className="min-w-0">
                <p className="text-[11px] font-bold uppercase tracking-[0.04em]">Result</p>
                <p className="text-[12px] font-medium">{concurrencyResult}</p>
              </div>
            </div>
          ) : (
            <p className="mt-3 text-meta text-muted">
              Exactly one replay should execute. The other 49 are duplicates.
            </p>
          )}
        </Group>
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        <Group title="Webhook replay" description="Re-send events through the pipeline.">
          <div className="flex flex-col gap-2">
            <Button
              variant="outline"
              className="w-full"
              onClick={() =>
                setAction({
                  title: 'Replay the last webhook?',
                  description:
                    'The most recent webhook event is sent through the pipeline again. No money moves.',
                  confirmLabel: 'Replay webhook',
                  run: () =>
                    simulate(
                      'Webhook replayed',
                      'The demo pipeline accepted the event again without a duplicate charge.',
                    ),
                })
              }
              icon={<RefreshCw className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
            >
              Replay last webhook
            </Button>
            <Button
              variant="outline"
              className="w-full"
              onClick={() =>
                setAction({
                  title: 'Send an invalid-signature webhook?',
                  description:
                    'FRACTAL should reject it and record the failed verification. No case is created.',
                  confirmLabel: 'Send invalid webhook',
                  destructive: true,
                  run: () =>
                    simulate(
                      'Invalid webhook sent',
                      'The demo pipeline rejected the signature and produced no case.',
                    ),
                })
              }
              icon={
                <AlertTriangle
                  className="h-4 w-4 text-[color:var(--danger-text)]"
                  aria-hidden="true"
                  strokeWidth={2.5}
                />
              }
            >
              Send invalid-signature webhook
            </Button>
          </div>
        </Group>

        <Group title="Seed scenarios" description="Create a case for a known failure.">
          <div className="flex flex-col gap-2">
            <Button
              variant="outline"
              className="w-full"
              onClick={() =>
                setAction(
                  seedAction(
                    'Amount mismatch',
                    'Creates a demo case where the payment and order totals differ.',
                  ),
                )
              }
            >
              Seed amount mismatch
            </Button>
            <Button
              variant="outline"
              className="w-full"
              onClick={() =>
                setAction(
                  seedAction(
                    'Currency mismatch',
                    'Creates a demo case where the payment and order currencies differ.',
                  ),
                )
              }
            >
              Seed currency mismatch
            </Button>
            <Button
              variant="outline"
              className="w-full"
              onClick={() =>
                setAction(
                  seedAction(
                    'Provider unavailable',
                    'Creates a demo case where the provider cannot confirm the payment.',
                  ),
                )
              }
            >
              Seed provider unavailable
            </Button>
          </div>
        </Group>
      </div>

      <div className="grid gap-5 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <Group
          title="Live webhook activity"
          description="Verified events for this merchant, refreshed every 10 seconds."
          aside={
            <span className="inline-flex items-center gap-1.5 rounded-pill border-2 border-line bg-success px-2 py-0.5 text-[11px] font-bold uppercase tracking-[0.04em] text-[color:var(--success-ink)]">
              <Radio className="h-3.5 w-3.5 animate-pulse" aria-hidden="true" strokeWidth={2.5} />
              Live
            </span>
          }
        >
          {liveLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-32 w-full" />
            </div>
          ) : liveError && !live ? (
            <ErrorNotice title="Live telemetry unavailable" description={liveError} />
          ) : live ? (
            <>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                {[
                  ['Events stored', live.webhook_events.total],
                  ['Last 24 hours', live.webhook_events.last_24h],
                ].map(([label, value]) => (
                  <div key={String(label)} className="rounded border-2 border-line bg-surface-alt p-3">
                    <p className="text-[28px] font-extrabold leading-8 tabular-nums text-ink">{value}</p>
                    <p className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">{label}</p>
                  </div>
                ))}
                <div className="rounded border-2 border-line bg-surface-alt p-3">
                  <p className="text-body font-extrabold leading-6 text-ink">
                    {live.webhook_events.last_received
                      ? formatAbsolute(live.webhook_events.last_received)
                      : 'No events yet'}
                  </p>
                  <p className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">Last received</p>
                </div>
              </div>

              <div className="mt-4 overflow-x-auto border-2 border-line">
                <table className="w-full min-w-[520px] border-collapse text-left">
                  <thead>
                    <tr className="border-b-2 border-line bg-surface-alt">
                      <th className="px-3 py-2 text-[11px] font-bold uppercase tracking-[0.04em] text-ink">Event</th>
                      <th className="px-3 py-2 text-[11px] font-bold uppercase tracking-[0.04em] text-ink">Signature</th>
                      <th className="px-3 py-2 text-[11px] font-bold uppercase tracking-[0.04em] text-ink">Status</th>
                      <th className="px-3 py-2 text-[11px] font-bold uppercase tracking-[0.04em] text-ink">Received</th>
                    </tr>
                  </thead>
                  <tbody>
                    {live.recent_events.length === 0 ? (
                      <tr>
                        <td colSpan={4} className="px-3 py-6 text-center text-meta text-muted">
                          No webhook events have arrived for this merchant yet.
                        </td>
                      </tr>
                    ) : (
                      live.recent_events.map((event) => (
                        <tr key={event.event_id} className="border-b-2 border-line last:border-b-0">
                          <td className="px-3 py-2">
                            <p className="truncate text-[12px] font-bold text-ink">{event.event_type}</p>
                            <p className="truncate font-mono text-[11px] text-muted">{event.event_id}</p>
                          </td>
                          <td className="px-3 py-2">
                            <StatusPill
                              size="sm"
                              meta={{
                                label: event.signature_verified ? 'Verified' : 'Rejected',
                                tone: event.signature_verified ? 'success' : 'danger',
                              }}
                            />
                          </td>
                          <td className="px-3 py-2 text-[12px] font-medium text-ink">
                            {sentenceCase(event.processing_status)}
                          </td>
                          <td className="px-3 py-2 text-[12px] text-muted">{formatAbsolute(event.received_at)}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </>
          ) : null}
        </Group>

        <Group title="Live Kibana" description="The real observability dashboard.">
          <div className="flex flex-1 flex-col gap-4">
            <p className="flex items-start gap-2 text-body text-ink">
              <Activity className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" strokeWidth={2.5} />
              Kibana reads the same OpenTelemetry logs and traces that FRACTAL writes. The mock charts are gone; this link
              opens the live stack.
            </p>

            {live?.provider_account && (
              <div className="rounded border-2 border-line bg-surface-alt p-3">
                <p className="meta-label">Merchant webhook</p>
                <div className="mt-1 flex flex-wrap items-center gap-2">
                  <StatusPill
                    size="sm"
                    meta={{
                      label: sentenceCase(live.provider_account.webhook_status),
                      tone:
                        live.provider_account.webhook_status === 'REGISTERED'
                          ? 'success'
                          : live.provider_account.webhook_status === 'FAILED'
                            ? 'danger'
                            : 'warning',
                    }}
                  />
                  <span className="font-mono text-[11px] text-muted">
                    {live.provider_account.webhook_id ?? 'No webhook ID'}
                  </span>
                </div>
              </div>
            )}

            <Button
              variant="secondary"
              className="w-full"
              onClick={() =>
                window.open(live?.kibana_url ?? 'http://localhost:5601', '_blank', 'noopener,noreferrer')
              }
              icon={<ExternalLink className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
            >
              Open live Kibana
            </Button>

            <p className="mt-auto text-[11px] leading-5 text-muted">
              {live?.generated_at
                ? `Telemetry snapshot ${formatAbsolute(live.generated_at)}.`
                : 'Waiting for the first telemetry snapshot.'}
            </p>
          </div>
        </Group>
      </div>

      <section className="panel flex flex-col gap-4 border-[color:var(--danger)] p-5 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <h2 className="text-body font-extrabold text-ink">Reset demo data</h2>
          <p className="mt-0.5 text-meta text-muted">
            Clears demo cases, payments and audit history. This cannot be undone.
          </p>
        </div>
        <Button
          variant="danger"
          className="w-full shrink-0 sm:w-auto"
          onClick={() =>
            setAction({
              title: 'Reset all demo data?',
              description:
                'Every demo case, payment and audit entry is removed and the seed data is restored. This cannot be undone.',
              confirmLabel: 'Reset demo data',
              destructive: true,
              run: () =>
                simulate(
                  'Demo data reset',
                  'The demo dataset is back to its starting state.',
                ),
            })
          }
          icon={<RotateCcw className="h-4 w-4" aria-hidden="true" strokeWidth={2.5} />}
        >
          Reset demo data
        </Button>
      </section>

      <ConfirmDialog
        open={action !== null}
        onClose={() => {
          if (!actionBusy) setAction(null);
        }}
        onConfirm={() => void runAction()}
        title={action?.title ?? ''}
        description={action?.description}
        confirmLabel={action?.confirmLabel ?? 'Confirm'}
        destructive={action?.destructive}
        loading={actionBusy}
      />
    </div>
  );
}
