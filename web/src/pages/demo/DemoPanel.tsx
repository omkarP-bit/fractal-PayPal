import { useState } from 'react';
import { LineChart, Line, ResponsiveContainer, BarChart, Bar, XAxis, YAxis, Tooltip, PieChart, Pie, Cell } from 'recharts';
import { Play, RefreshCw, AlertTriangle, CheckCircle, Server, AlertCircle } from '../../components/ui/Icons';

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
  { name: 'ALLOW', value: 65, color: '#22c55e' },
  { name: 'REQUIRE_APPROVAL', value: 20, color: '#2563eb' },
  { name: 'WAIT', value: 10, color: '#f59e0b' },
  { name: 'BLOCK', value: 5, color: '#ef4444' },
];

export function DemoPanel() {
  const [handlerBroken, setHandlerBroken] = useState(false);
  const [concurrencyResult, setConcurrencyResult] = useState<string | null>(null);
  const [isRunning, setIsRunning] = useState(false);

  const toggleHandler = () => setHandlerBroken(!handlerBroken);

  const runConcurrencyTest = async () => {
    setIsRunning(true);
    setConcurrencyResult(null);
    await new Promise((resolve) => setTimeout(resolve, 2000));
    setConcurrencyResult('1 executed, 49 duplicates');
    setIsRunning(false);
  };

  const renderButtonContent = () => {
    if (isRunning) {
      return (
        <span className="flex items-center gap-2">
          <Play className="w-4 h-4 animate-spin" />
          Running...
        </span>
      );
    }
    return (
      <span className="flex items-center gap-2">
        <Play className="w-4 h-4" />
        Fire 50 Concurrent Replays
      </span>
    );
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Demo Control Panel</h1>
          <p className="text-gray-600 text-sm">Local-only demo controls for testing FRACTAL flows</p>
        </div>
        <div className="flex items-center gap-3">
          <span className={`px-2 py-1 rounded text-xs font-medium ${
            handlerBroken ? 'bg-red-50 text-red-600' : 'bg-green-50 text-green-600'
          }`}>
            {handlerBroken ? 'Handler: BROKEN' : 'Handler: Working'}
          </span>
          <button
            onClick={toggleHandler}
            className="px-3 py-1.5 rounded text-sm font-medium text-gray-600 bg-white border border-gray-200 hover:bg-gray-100 transition-colors"
          >
            Toggle Handler
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-medium text-gray-900">Webhook Replay</h3>
            <Server className="w-4 h-4 text-gray-400" />
          </div>
          <div className="space-y-3">
            <button
              className="w-full flex items-center justify-center gap-2 px-4 py-3 border border-gray-200 rounded-lg text-sm text-gray-700 hover:bg-gray-50 transition-colors"
            >
              <RefreshCw className="w-4 h-4" /> Replay Last Webhook
            </button>
            <button
              className="w-full flex items-center justify-center gap-2 px-4 py-3 border border-gray-200 rounded-lg text-sm text-gray-700 hover:bg-gray-50 transition-colors"
            >
              <AlertCircle className="w-4 h-4 text-red-500" />
              Send Invalid Signature Webhook
            </button>
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-medium text-gray-900">Concurrency Test</h3>
            <CheckCircle className="w-4 h-4 text-gray-400" />
          </div>
          <div className="space-y-3">
            <div className="text-sm text-gray-500">Fire 50 concurrent webhook replays</div>
            <button
              onClick={runConcurrencyTest}
              disabled={isRunning}
              className={`w-full flex items-center justify-center gap-2 px-4 py-3 rounded-lg text-sm font-medium transition-colors ${
                isRunning ? 'bg-gray-200 text-gray-400 cursor-not-allowed' : 'bg-paypal-600 text-white hover:bg-paypal-700'
              }`}
            >
              {renderButtonContent()}
            </button>
            {concurrencyResult && (
              <div className="bg-green-50 rounded-lg p-3 border border-green-200">
                <div className="text-xs font-medium text-green-700 mb-1">Result</div>
                <div className="text-sm text-green-700 font-medium">{concurrencyResult}</div>
              </div>
            )}
          </div>
        </div>

        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h3 className="font-medium text-gray-900">Seed Scenarios</h3>
            <AlertTriangle className="w-4 h-4 text-gray-400" />
          </div>
          <div className="space-y-3">
            <button
              className="w-full flex items-center justify-center gap-2 px-4 py-3 border border-gray-200 rounded-lg text-sm text-gray-700 hover:bg-gray-50 transition-colors"
            >
              <AlertCircle className="w-4 h-4" />
              Seed Amount Mismatch
            </button>
            <button
              className="w-full flex items-center justify-center gap-2 px-4 py-3 border border-gray-200 rounded-lg text-sm text-gray-700 hover:bg-gray-50 transition-colors"
            >
              <AlertCircle className="w-4 h-4 text-orange-500" />
              Seed Currency Mismatch
            </button>
            <button
              className="w-full flex items-center justify-center gap-2 px-4 py-3 border border-gray-200 rounded-lg text-sm text-gray-700 hover:bg-gray-50 transition-colors"
            >
              <AlertCircle className="w-4 h-4 text-red-500" />
              Seed Provider Unavailable
            </button>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="card lg:col-span-1">
          <h3 className="font-medium text-gray-900 mb-4">Policy Decisions (Last 24h)</h3>
          <div className="h-64">
            <PieChart>
              <Pie
                data={MOCK_DISTRIBUTION}
                dataKey="value"
                nameKey="name"
                cx="50%"
                cy="50%"
                outerRadius={80}
                innerRadius={60}
                labelLine={false}
                label={({ name, percent }: any) => (
                  <text x={0} y={6} dy={6} textAnchor="middle" fontSize={12} fill="#666">
                    {name} {percent}%
                  </text>
                )}
              >
                {MOCK_DISTRIBUTION.map((entry) => (
                  <Cell key={entry.name} fill={entry.color} />
                ))}
              </Pie>
            </PieChart>
          </div>
          <div className="flex justify-between text-xs text-gray-500 mt-3">
            {MOCK_DISTRIBUTION.map((d) => (
              <span key={d.name} className="flex items-center gap-1">
                <span className={`w-2 h-2 rounded-full ${getColor(d.color)}`} />
                {d.name}
              </span>
            ))}
          </div>
        </div>

        <div className="card lg:col-span-2">
          <h3 className="font-medium text-gray-900 mb-4">Webhook Latency by Stage</h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={MOCK_LATENCY_DATA}>
                <XAxis dataKey="name" tick={{ fontSize: 10 }} tickLine={false} />
                <YAxis tick={{ fontSize: 10 }} tickLine={false} />
                <Tooltip />
                <Line type="monotone" dataKey="latency" stroke="#2563eb" strokeWidth={2} dot />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>

        <div className="card lg:col-span-2">
          <h3 className="font-medium text-gray-900 mb-4">Concurrent Replay Success Rate</h3>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={MOCK_REPLAY_DATA}>
                <XAxis dataKey="name" tick={{ fontSize: 10 }} tickLine={false} />
                <YAxis tick={{ fontSize: 10 }} tickLine={false} />
                <Tooltip />
                <Bar dataKey="success" fill="#22c55e" radius={[4, 4, 0, 0]} />
                <Bar dataKey="failed" fill="#ef4444" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="flex justify-between text-xs text-gray-500 mt-3">
            <span className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full bg-green-500" /> Success
            </span>
            <span className="flex items-center gap-1">
              <span className="w-2 h-2 rounded-full bg-red-500" /> Failed
            </span>
          </div>
        </div>
      </div>

      <div className="card">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="font-medium text-gray-900">Reset Demo Data</h3>
            <p className="text-sm text-gray-500">Reset all cases, transactions, and audit trail to initial state</p>
          </div>
          <button className="px-4 py-2 rounded-lg text-sm font-medium text-red-600 bg-red-50 hover:bg-red-100 transition-colors">
            Reset All Demo Data
          </button>
        </div>
      </div>
    </div>
  );
}

const getColor = (color: string) => {
  if (color === '#22c55e') return 'bg-green-500';
  if (color === '#2563eb') return 'bg-blue-500';
  if (color === '#f59e0b') return 'bg-amber-500';
  return 'bg-red-500';
}
