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
import { PageHeader, PanelHeader } from '../../components/ui/Panel';
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

function ChartPanel({
  title,
  description,
  children,
  className,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn('rounded-lg border border-line bg-surface p-5', className)}>
      <PanelHeader title={title} description={description} />
      <div className="mt-4">{children}</div>
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
    <div className="space-y-5">
      <PageHeader
        title="Demo controls"
        description="Simulated, local-only tools for testing FRACTAL flows."
      />

      <div className="grid gap-5 lg:grid-cols-3">
        <section className="rounded-lg border border-line bg-surface p-5 lg:col-span-2">
          <PanelHeader
            title="Webhook handling"
            description="Control how the mock store responds to payment notifications."
            aside={
              <StatusPill
                meta={
                  handlerBroken
                    ? { label: 'Handler broken', tone: 'danger' }
                    : { label: 'Handler working', tone: 'success' }
                }
              />
            }
          />
          <div className="mt-4 flex flex-wrap items-center gap-3 border-t border-line pt-4">
            <Server className="h-4 w-4 text-muted" aria-hidden="true" strokeWidth={1.75} />
            <p className="min-w-0 flex-1 text-body text-muted">
              {handlerBroken
                ? 'Webhook deliveries are failing. Restore the handler to resume the demo flow.'
                : 'Webhook deliveries are processed normally. Break it to test failure handling.'}
            </p>
            <Button
              variant={handlerBroken ? 'secondary' : 'danger'}
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
        </section>

        <section className="rounded-lg border border-line bg-surface p-5">
          <PanelHeader
            title="Concurrency test"
            description="Fire 50 replays at the same time."
          />
          <Button
            variant="primary"
            className="mt-4 w-full"
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
            icon={<FlaskConical className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
          >
            Run 50 concurrent replays
          </Button>
          {concurrencyResult && (
            <div className="mt-3 flex items-start gap-2 rounded-md bg-success-bg p-3">
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" aria-hidden="true" />
              <div>
                <p className="text-meta font-medium text-success">Result</p>
                <p className="text-[12px] text-ink">{concurrencyResult}</p>
              </div>
            </div>
          )}
        </section>
      </div>

      <div className="grid gap-5 md:grid-cols-2">
        <section className="rounded-lg border border-line bg-surface p-5">
          <PanelHeader title="Webhook replay" description="Re-send events through the pipeline." />
          <div className="mt-4 space-y-2">
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
              icon={<RefreshCw className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
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
              icon={<AlertTriangle className="h-4 w-4 text-danger" aria-hidden="true" strokeWidth={1.75} />}
            >
              Send invalid-signature webhook
            </Button>
          </div>
        </section>

        <section className="rounded-lg border border-line bg-surface p-5">
          <PanelHeader title="Seed scenarios" description="Create a case for a known failure." />
          <div className="mt-4 space-y-2">
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
        </section>
      </div>

      <div className="grid gap-5 lg:grid-cols-3">
        <ChartPanel title="Policy decisions" description="Last 24 hours">
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
                  stroke="var(--surface)"
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
              <span className="text-section font-semibold tabular-nums text-ink">100</span>
              <span className="text-[11px] text-muted">decisions</span>
            </div>
          </div>
          <ul className="mt-2 space-y-1.5">
            {MOCK_DISTRIBUTION.map((entry) => (
              <li key={entry.name} className="flex items-center justify-between gap-3 text-[12px]">
                <span className="flex items-center gap-1.5 text-muted">
                  <span
                    className="h-1.5 w-1.5 rounded-full"
                    style={{ backgroundColor: entry.color }}
                    aria-hidden="true"
                  />
                  {entry.name}
                </span>
                <span className="tabular-nums text-ink">{entry.value}%</span>
              </li>
            ))}
          </ul>
        </ChartPanel>

        <ChartPanel title="Webhook latency" description="Milliseconds by pipeline stage" className="lg:col-span-2">
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={MOCK_LATENCY_DATA} margin={{ top: 8, right: 8, left: -18, bottom: 0 }}>
                <CartesianGrid stroke="var(--border)" vertical={false} />
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
                  strokeWidth={2}
                  dot={{ r: 3, fill: 'var(--brand-blue)' }}
                  activeDot={{ r: 4 }}
                />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </ChartPanel>
      </div>

      <ChartPanel
        title="Concurrent replay success rate"
        description="Five runs of 50 simultaneous webhook replays"
      >
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={MOCK_REPLAY_DATA} margin={{ top: 8, right: 8, left: -18, bottom: 0 }}>
              <CartesianGrid stroke="var(--border)" vertical={false} />
              <XAxis dataKey="name" tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
              <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} />
              <ChartTooltip />
              <Bar dataKey="success" name="Executed" fill="var(--success)" radius={[4, 4, 0, 0]} />
              <Bar dataKey="failed" name="Blocked" fill="var(--danger)" radius={[4, 4, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>
        <div className="mt-3 flex items-center gap-4 text-[12px] text-muted">
          <span className="flex items-center gap-1.5">
            <span className="h-1.5 w-1.5 rounded-full bg-success" aria-hidden="true" />
            Executed
          </span>
          <span className="flex items-center gap-1.5">
            <span className="h-1.5 w-1.5 rounded-full bg-danger" aria-hidden="true" />
            Blocked
          </span>
        </div>
      </ChartPanel>

      <section className="flex flex-col gap-4 rounded-lg border border-danger bg-surface p-5 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-body font-semibold text-ink">Reset demo data</h2>
          <p className="mt-0.5 text-meta text-muted">
            Clears demo cases, payments and audit history. This cannot be undone.
          </p>
        </div>
        <Button
          variant="danger"
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
          icon={<RotateCcw className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />}
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
