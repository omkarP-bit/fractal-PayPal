import { useState, type ReactNode } from 'react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  XAxis,
  YAxis,
} from 'recharts';
import {
  CheckCircle2,
  FlaskConical,
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

const MOCK_REPLAY_DATA = [
  { name: 'Attempt 1', success: 45, failed: 5 },
  { name: 'Attempt 2', success: 44, failed: 6 },
  { name: 'Attempt 3', success: 46, failed: 4 },
  { name: 'Attempt 4', success: 45, failed: 5 },
  { name: 'Attempt 5', success: 47, failed: 3 },
];

const MOCK_LATENCY_DATA = [
  { name: 'Webhook', latency: 120 },
  { name: 'Verify', latency: 85 },
  { name: 'Store', latency: 45 },
  { name: 'Process', latency: 30 },
  { name: 'Execute', latency: 150 },
];

const MOCK_DISTRIBUTION = [
  { name: 'Approved', value: 65, color: 'var(--success)' },
  { name: 'Needs approval', value: 20, color: 'var(--warning)' },
  { name: 'Waiting', value: 10, color: 'var(--info)' },
  { name: 'Blocked', value: 5, color: 'var(--danger)' },
];

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
        description="Simulated, local-only tools for testing FRACTAL flows."
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

      <div className="grid gap-5 lg:grid-cols-3">
        <Group title="Policy decisions" description="Last 24 hours">
          <div className="relative h-52">
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={MOCK_DISTRIBUTION}
                  dataKey="value"
                  nameKey="name"
                  cx="50%"
                  cy="50%"
                  innerRadius={54}
                  outerRadius={82}
                  stroke="var(--ink)"
                  strokeWidth={2}
                >
                  {MOCK_DISTRIBUTION.map((entry) => (
                    <Cell key={entry.name} fill={entry.color} />
                  ))}
                </Pie>
                <ChartTooltip />
              </PieChart>
            </ResponsiveContainer>
            <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
              <span className="text-[32px] font-extrabold leading-9 tabular-nums text-ink">100</span>
              <span className="text-[11px] font-bold uppercase tracking-[0.04em] text-muted">
                decisions
              </span>
            </div>
          </div>
          <ul className="mt-3 space-y-1.5 border-t-2 border-line pt-3">
            {MOCK_DISTRIBUTION.map((entry) => (
              <li key={entry.name} className="flex items-center justify-between gap-3 text-[12px]">
                <span className="flex items-center gap-2 text-ink">
                  <span
                    className="h-2.5 w-2.5 border-2 border-line"
                    style={{ backgroundColor: entry.color }}
                    aria-hidden="true"
                  />
                  {entry.name}
                </span>
                <span className="font-mono font-bold tabular-nums text-ink">{entry.value}%</span>
              </li>
            ))}
          </ul>
        </Group>

        <Group
          title="Webhook latency"
          description="Milliseconds by pipeline stage"
          className="lg:col-span-2"
        >
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={MOCK_LATENCY_DATA} margin={{ top: 8, right: 8, left: -18, bottom: 0 }}>
                <CartesianGrid stroke="var(--border-color)" strokeDasharray="3 3" vertical={false} />
                <XAxis
                  dataKey="name"
                  tick={{ fontSize: 11 }}
                  tickLine={false}
                  axisLine={false}
                />
                <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
                <ChartTooltip />
                <Line
                  type="monotone"
                  dataKey="latency"
                  stroke="var(--brand-blue)"
                  strokeWidth={3}
                  dot={{ r: 3, fill: 'var(--brand-blue)', stroke: 'var(--ink)', strokeWidth: 2 }}
                  activeDot={{ r: 4 }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </Group>
      </div>

      <Group
        title="Concurrent replay success rate"
        description="Five runs of 50 simultaneous webhook replays"
      >
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={MOCK_REPLAY_DATA} margin={{ top: 8, right: 8, left: -18, bottom: 0 }}>
              <CartesianGrid stroke="var(--border-color)" strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="name" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
              <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
              <ChartTooltip />
              <Bar dataKey="success" name="Executed" fill="var(--success)" stroke="var(--ink)" strokeWidth={2} />
              <Bar dataKey="failed" name="Blocked" fill="var(--danger)" stroke="var(--ink)" strokeWidth={2} />
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-3 flex items-center gap-4 text-[12px] font-bold text-ink">
          <span className="flex items-center gap-1.5">
            <span className="h-2.5 w-2.5 border-2 border-line bg-success" aria-hidden="true" />
            Executed
          </span>
          <span className="flex items-center gap-1.5">
            <span className="h-2.5 w-2.5 border-2 border-line bg-danger" aria-hidden="true" />
            Blocked
          </span>
        </div>
      </Group>

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
