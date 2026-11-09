import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ArrowLeft, Mail, Lock, CreditCard, Plus } from '../../components/ui/Icons';
import { apiClient, CreateCaseRequest } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { Button } from '../../components/ui/Button';
import { Input } from '../../components/ui/Input';

export function NewCase() {
  const [form, setForm] = useState({
    merchant_id: '',
    merchant_order_ref: '',
    email: '',
    token: '',
    case_type: 'PROVIDER_MERCHANT_STATE_MISMATCH',
    description: '',
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();
  const { user } = useAuthStore();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      await apiClient.createCase({
        merchant_id: form.merchant_id,
        merchant_order_ref: form.merchant_order_ref,
        claim: { email: form.email, token: form.token },
        case_type: form.case_type,
        description: form.description,
      });
      navigate('/customer/cases');
    } catch (err: any) {
      setError(err.message || 'Failed to create case');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto py-6">
      <div className="flex items-center gap-3 mb-6">
        <button
          onClick={() => navigate('/customer/cases')}
          className="p-2 rounded-md text-gray-400 hover:bg-gray-100 hover:text-gray-600 transition-colors"
        >
          <ArrowLeft className="w-4 h-4" />
        </button>
        <h1 className="text-xl font-bold text-gray-900">Report Payment Issue</h1>
      </div>

      {error && (
        <div className="mb-4 bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-700">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-6">
        <div className="bg-white dark:bg-gray-800 border border-gray-200 rounded-lg shadow-sm p-6">
          <div className="flex items-center gap-2 mb-4">
            <CreditCard className="w-4 h-4 text-gray-500" />
            <h3 className="font-medium text-gray-900">Payment Details</h3>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Merchant ID
              </label>
              <input
                type="text"
                value={form.merchant_id}
                onChange={(e) => setForm({ ...form, merchant_id: e.target.value })}
                placeholder="MER-1234"
                className="input-field"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Order Reference
              </label>
              <input
                type="text"
                value={form.merchant_order_ref}
                onChange={(e) => setForm({ ...form, merchant_order_ref: e.target.value })}
                placeholder="ORDER-1234"
                className="input-field"
                required
              />
            </div>
          </div>
        </div>

        <div className="bg-white dark:bg-gray-800 border border-gray-200 rounded-lg shadow-sm p-6">
          <div className="flex items-center gap-2 mb-4">
            <Lock className="w-4 h-4 text-gray-500" />
            <h3 className="font-medium text-gray-900">Ownership Proof</h3>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Email Address
              </label>
              <input
                type="email"
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
                placeholder="buyer@example.com"
                className="input-field"
                required
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">
                Claim Code
              </label>
              <input
                type="text"
                value={form.token}
                onChange={(e) => setForm({ ...form, token: e.target.value })}
                placeholder="K7Q2-M9XD"
                className="input-field"
                required
              />
              <p className="text-xs text-gray-500 mt-1">
                Found on your order confirmation page
              </p>
            </div>
          </div>
        </div>

        <div className="bg-white dark:bg-gray-800 border border-gray-200 rounded-lg shadow-sm p-6">
          <div className="flex items-center gap-2 mb-4">
            <Mail className="w-4 h-4 text-gray-500" />
            <h3 className="font-medium text-gray-900">Description</h3>
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              What happened?
            </label>
            <textarea
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              rows={4}
              placeholder="I was charged but the store says unpaid..."
              className="input-field"
              required
            ></textarea>
          </div>
        </div>

        <button
          type="submit"
          disabled={loading}
          className="btn-primary w-full flex items-center justify-center gap-2 py-3"
        >
          <Plus className="w-4 h-4" /> Open Case
        </button>
      </form>
    </div>
  );
}
