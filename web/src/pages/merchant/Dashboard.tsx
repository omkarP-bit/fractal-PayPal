import { useState, useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Clock, ShieldCheck, CreditCard, AlertCircle, Bell, Filter } from '../../components/ui/Icons';
import { apiClient, Case } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { Card } from '../components/ui/Card';
import { Button } from '../components/ui/Button';
import { Badge } from '../components/ui/Badge';

interface MerchantCase extends Case {
  investigation?: {
    category: string;
    root_cause_hypothesis: string;
    suggested_severity: string;
  };
  latest_decision?: { outcome: string };
}

export function MerchantDashboard() {
  const [cases, setCases] = useState<MerchantCase[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState('all');
  const [search, setSearch] = useState('');
  const { logout } = useAuthStore();
  const navigate = useNavigate();

  const fetchCases = async () => {
    try {
      setLoading(true);
      const data = await apiClient.getCases();
      setCases(data as MerchantCase[]);
    } catch (err: any) {
      setError(err.message || 'Failed to load cases');
      if (err.message.includes('401')) {
        logout();
        navigate('/login');
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCases();
  }, []);

  const filteredCases = cases.filter((c) => {
    const matchesFilter =
      filter === 'all' ||
      (filter === 'needs-approval' && c.status === 'AWAITING_MERCHANT_APPROVAL') ||
      (filter === 'investigating' && c.status === 'INVESTIGATING') ||
      (filter === 'escalated' && c.status === 'ESCALATED');
    const matchesSearch =
      c.case_number.toLowerCase().includes(search.toLowerCase()) ||
      (c.customer?.email && c.customer.email.includes(search.toLowerCase())) ||
      (c.order?.ref && c.order.ref.includes(search.toLowerCase()));
    return matchesFilter && matchesSearch;
  });

  const handleCaseClick = (id: string) => {
    navigate(`/merchant/cases/${id}`);
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="flex flex-col items-center gap-3">
          <div className="w-8 h-8 border-4 border-paypal-600 border-t-transparent rounded-full animate-spin" />
          <p className="text-gray-500 text-sm">Loading cases...</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-64 text-center">
        <div className="w-12 h-12 rounded-full bg-red-50 flex items-center justify-center">
          <AlertCircle className="w-6 h-6 text-red-500" />
        </div>
        <h3 className="text-lg font-medium text-gray-900 mt-4">Error loading cases</h3>
        <p className="text-sm text-gray-600">{error}</p>
        <button
          onClick={() => fetchCases()}
          className="mt-4 btn-primary px-6 py-2"
        >
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-gray-900">Cases</h1>
          <p className="text-gray-600 text-sm">
            Manage payment exception cases for your store
          </p>
        </div>
        <div className="flex items-center gap-3">
          <input
            type="text"
            placeholder="Search cases..."
            className="input-field w-64"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <button className="btn-secondary px-3 py-2">
            <Bell className="w-4 h-4" />
          </button>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-xs font-medium text-gray-500 uppercase">Total Cases</p>
              <p className="text-2xl font-bold text-gray-900">{cases.length}</p>
            </div>
            <div className="w-10 h-10 rounded-lg bg-blue-50 flex items-center justify-center">
              <CreditCard className="w-5 h-5 text-blue-600" />
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-xs font-medium text-gray-500 uppercase">Investigating</p>
              <p className="text-2xl font-bold text-gray-900">
                {cases.filter((c) => c.status === 'INVESTIGATING').length}
              </p>
            </div>
            <div className="w-10 h-10 rounded-lg bg-blue-50 flex items-center justify-center">
              <Clock className="w-5 h-5 text-blue-600" />
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-xs font-medium text-gray-500 uppercase">Needs Approval</p>
              <p className="text-2xl font-bold text-gray-900">
                {cases.filter((c) => c.status === 'AWAITING_MERCHANT_APPROVAL').length}
              </p>
            </div>
            <div className="w-10 h-10 rounded-lg bg-amber-50 flex items-center justify-center">
              <Bell className="w-5 h-5 text-amber-600" />
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-xs font-medium text-gray-500 uppercase">Escalated</p>
              <p className="text-2xl font-bold text-gray-900">
                {cases.filter((c) => c.status === 'ESCALATED').length}
              </p>
            </div>
            <div className="w-10 h-10 rounded-lg bg-red-50 flex items-center justify-center">
              <AlertCircle className="w-5 h-5 text-red-600" />
            </div>
          </div>
        </div>
      </div>

      <div className="flex gap-2">
        {['all', 'needs-approval', 'investigating', 'escalated'].map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={`px-3 py-1 rounded-md text-sm font-medium transition-colors ${
              filter === f
                ? 'bg-paypal-600 text-white'
                : 'bg-white text-gray-600 border border-gray-200 hover:bg-gray-50'
            }`}
          >
            {f.charAt(0).toUpperCase() + f.slice(1).replace('-', ' ')}
          </button>
        ))}
      </div>

      <div className="card">
        <div className="overflow-hidden">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50">
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Case</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Customer</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Order</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Type</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Updated</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {filteredCases.map((c) => (
                <tr
                  key={c.id}
                  onClick={() => handleCaseClick(c.id)}
                  className="cursor-pointer hover:bg-gray-50 transition-colors group"
                >
                  <td className="px-6 py-4">
                    <div className="font-medium text-gray-900">{c.case_number}</div>
                    <div className="text-xs text-gray-500">{c.id.slice(0, 8)}...</div>
                  </td>
                  <td className="px-6 py-4">
                    <div className="font-medium text-gray-900">{c.customer?.name || 'Unknown'}</div>
                    <div className="text-xs text-gray-500">{c.customer?.email || '-'}</div>
                  </td>
                  <td className="px-6 py-4">
                    <div className="font-medium text-gray-900">{c.order?.ref || '-'}</div>
                    <div className="text-xs text-gray-500">{c.order?.status || '-'}</div>
                  </td>
                  <td className="px-6 py-4">
                    <span className="text-xs text-gray-500">{c.case_type?.replace('_', ' ') || '-'}</span>
                  </td>
                  <td className="px-6 py-4">
                    <span className={`badge badge-${c.status.toLowerCase()}`}>
                      {c.status.replace('_', ' ')}
                    </span>
                  </td>
                  <td className="px-6 py-4 text-right text-sm text-gray-500">
                    {new Date(c.updated_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
              {filteredCases.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-6 py-12 text-center">
                    <div className="flex flex-col items-center gap-3">
                      <p className="text-gray-500 text-sm">No cases found</p>
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
