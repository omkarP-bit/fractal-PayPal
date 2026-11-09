import { useState, useEffect } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { Plus, Clock, CreditCard, AlertCircle, Info, ShieldCheck } from '../../components/ui/Icons';
import { apiClient } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { Badge } from '../../components/ui/Badge';
import { Button } from '../../components/ui/Button';

interface Case {
  id: string;
  case_number: string;
  status: string;
  merchant: { id: string; name: string };
  payment: { status: string; amount_minor: number; currency: string };
  order: { ref: string; status: string };
  summary: string;
  next_step: string;
  version: number;
  updated_at: string;
}

export function CustomerDashboard() {
  const [cases, setCases] = useState<Case[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  const fetchCases = async () => {
    try {
      setLoading(true);
      const data = await apiClient.getCases();
      setCases(data);
    } catch (err: any) {
      setError(err.message || 'Failed to load cases');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCases();
  }, []);

  const handleNewCase = () => {
    navigate('/customer/cases/new');
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'COMPLETED': return 'badge-resolved';
      case 'PAYMENT_PENDING': return 'badge-pending';
      case 'INVESTIGATING': return 'badge-investigating';
      case 'RESOLVED': return 'badge-resolved';
      case 'ESCALATED': return 'badge-escalated';
      default: return 'badge-pending';
    }
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
          <h1 className="text-2xl font-bold text-gray-900">Your Cases</h1>
          <p className="text-gray-600 text-sm">
            Track the status of your payment exception cases
          </p>
        </div>
        <button
          onClick={handleNewCase}
          className="btn-primary flex items-center gap-2 px-5 py-2.5 text-sm font-medium"
        >
          <Plus className="w-4 h-4" /> Report Payment Issue
        </button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="bg-white dark:bg-gray-800 border border-gray-200 rounded-lg shadow-sm p-6">
          <div className="flex items-end gap-2">
            <span className="text-3xl font-bold text-gray-900">{cases.length}</span>
          </div>
          <p className="text-xs font-medium text-gray-500 uppercase mt-3">Total Cases</p>
        </div>
        <div className="bg-white dark:bg-gray-800 border border-gray-200 rounded-lg shadow-sm p-6">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-8 h-8 rounded-lg bg-blue-50 flex items-center justify-center">
              <Clock className="w-4 h-4 text-blue-600" />
            </div>
            <div>
              <p className="text-xs font-medium text-gray-500 uppercase">Investigating</p>
              <p className="text-2xl font-bold text-gray-900">{cases.filter((c) => c.status === 'INVESTIGATING').length}</p>
            </div>
          </div>
        </div>
        <div className="bg-white dark:bg-gray-800 border border-gray-200 rounded-lg shadow-sm p-6">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-8 h-8 rounded-lg bg-green-50 flex items-center justify-center">
              <ShieldCheck className="w-4 h-4 text-green-600" />
            </div>
            <div>
              <p className="text-xs font-medium text-gray-500 uppercase">Resolved</p>
              <p className="text-2xl font-bold text-gray-900">{cases.filter((c) => c.status === 'RESOLVED').length}</p>
            </div>
          </div>
        </div>
      </div>

      <div className="bg-white dark:bg-gray-800 border border-gray-200 rounded-lg shadow-sm">
        <div className="overflow-hidden">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-gray-200 bg-gray-50">
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Case</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Merchant</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Payment</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Order</th>
                <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase">Status</th>
                <th className="px-6 py-3 text-right text-xs font-medium text-gray-500 uppercase">Updated</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-gray-200">
              {cases.map((c) => (
                <tr
                  key={c.id}
                  onClick={() => navigate(`/customer/cases/${c.id}`)}
                  className="cursor-pointer hover:bg-gray-50 transition-colors group"
                >
                  <td className="px-6 py-4">
                    <div className="font-medium text-gray-900">{c.case_number}</div>
                    <div className="text-xs text-gray-500">{c.id.slice(0, 8)}...</div>
                  </td>
                  <td className="px-6 py-4">
                    <div className="font-medium text-gray-900">{c.merchant?.name || 'Unknown'}</div>
                  </td>
                  <td className="px-6 py-4">
                    <div className="flex items-center gap-2">
                      <span className={`badge ${caseStatusColor(c.payment?.status || 'COMPLETED')}`}>
                        {c.payment?.status || 'Unknown'}
                      </span>
                      <div className="text-sm text-gray-900">
                        ${(c.payment?.amount_minor ? (c.payment.amount_minor / 100).toFixed(2) : '0.00')} {c.payment?.currency || 'USD'}
                      </div>
                    </div>
                  </td>
                  <td className="px-6 py-4">
                    <div className="font-medium text-gray-900">{c.order?.ref || '-'}</div>
                    <div className="text-xs text-gray-500">{c.order?.status || '-'}</div>
                  </td>
                  <td className="px-6 py-4">
                    <span className={`badge ${caseStatusColor(c.status)}`}>
                      {c.status.replace('_', ' ')}
                    </span>
                  </td>
                  <td className="px-6 py-4 text-right text-sm text-gray-500">
                    {new Date(c.updated_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
              {cases.length === 0 && (
                <tr>
                  <td colSpan={6} className="px-6 py-12 text-center">
                    <div className="flex flex-col items-center gap-3">
                      <div className="w-12 h-12 rounded-full bg-gray-100 flex items-center justify-center">
                        <Info className="w-6 h-6 text-gray-400" />
                      </div>
                      <p className="text-gray-500 text-sm">No cases found</p>
                      <p className="text-xs text-gray-400">Report a payment issue to get started</p>
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="bg-blue-50 border border-blue-100 rounded-lg p-6">
        <div className="flex items-start gap-3">
          <Info className="w-5 h-5 text-blue-600 mt-0.5" />
          <div>
            <h3 className="font-medium text-blue-900">Need help?</h3>
            <p className="text-sm text-blue-800 mt-1">
              If you were charged but your order hasn't updated, open a case using your claim code from the confirmation page.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

const caseStatusColor = (status: string) => {
  switch (status) {
    case 'COMPLETED': return 'badge-resolved';
    case 'PAYMENT_PENDING': return 'badge-pending';
    case 'INVESTIGATING': return 'badge-investigating';
    case 'RESOLVED': return 'badge-resolved';
    case 'ESCALATED': return 'badge-escalated';
    default: return 'badge-pending';
  }
};
