import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, ShieldCheck, Clock, MessageCircle, FileText } from '../../components/ui/Icons';
import { apiClient, CustomerCaseView } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { useSSE } from '../../hooks/useSSE';
import { Badge } from '../components/ui/Badge';

export function CustomerCaseDetail() {
  const { id } = useParams();
  const [caseData, setCaseData] = useState<CustomerCaseView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [newMessage, setNewMessage] = useState('');
  const { logout } = useAuthStore();
  const navigate = useNavigate();

  const fetchCase = async () => {
    try {
      setLoading(true);
      const data = await apiClient.getCase(id || '');
      setCaseData(data as CustomerCaseView);
    } catch (err: any) {
      setError(err.message || 'Failed to load case');
      if (err.message.includes('401')) {
        logout();
        navigate('/login');
      }
    } finally {
      setLoading(false);
    }
  };

  const handleMessageEvent = (event: { type: string; case_id: string; version: number }) => {
    fetchCase();
  };

  useEffect(() => {
    fetchCase();
  }, [id]);

  useSSE(id || '', handleMessageEvent);

  const handleSendMessage = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newMessage.trim() || !id) return;

    try {
      await apiClient.createCaseMessage(id, newMessage, 'SHARED');
      setNewMessage('');
      fetchCase();
    } catch (err: any) {
      setError(err.message || 'Failed to send message');
    }
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'COMPLETED': return 'bg-green-50 text-green-700 border-green-200';
      case 'PAYMENT_PENDING': return 'bg-amber-50 text-amber-700 border-amber-200';
      case 'INVESTIGATING': return 'bg-blue-50 text-blue-700 border-blue-200';
      case 'RESOLVED': return 'bg-green-50 text-green-700 border-green-200';
      case 'ESCALATED': return 'bg-red-50 text-red-700 border-red-200';
      default: return 'bg-gray-50 text-gray-700 border-gray-200';
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="flex flex-col items-center gap-3">
          <div className="w-8 h-8 border-4 border-paypal-600 border-t-transparent rounded-full animate-spin" />
          <p className="text-gray-500 text-sm">Loading case...</p>
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
        <h3 className="text-lg font-medium text-gray-900 mt-4">Error loading case</h3>
        <p className="text-sm text-gray-600">{error}</p>
        <button
          onClick={() => navigate('/customer/cases')}
          className="mt-4 btn-primary px-6 py-2"
        >
          Back to Cases
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div className="flex items-center gap-3">
          <button
            onClick={() => navigate('/customer/cases')}
            className="p-2 rounded-md text-gray-400 hover:bg-gray-100 hover:text-gray-600 transition-colors"
          >
            <ArrowLeft className="w-4 h-4" />
          </button>
          <div>
            <h1 className="text-xl font-bold text-gray-900">Case {caseData.case_number}</h1>
            <p className="text-sm text-gray-500">{caseData.status.replace('_', ' ')}</p>
          </div>
        </div>
        <span className={`badge badge-investigating`}>
          {caseData.status.replace('_', ' ')}
        </span>
      </div>

      <div className="card">
        <div className="flex items-center justify-between py-4">
          <div className="flex-1 flex items-center justify-center">
            <div className="w-full h-0.5 bg-gray-200"></div>
          </div>
          <div className="w-8 h-8 rounded-full bg-paypal-600 flex items-center justify-center text-white text-sm font-medium">
            1
          </div>
          <div className="flex-1 flex items-center justify-center">
            <div className="w-full h-0.5 bg-gray-200"></div>
          </div>
          <div className="w-8 h-8 rounded-full bg-paypal-600 flex items-center justify-center text-white text-sm font-medium">
            2
          </div>
          <div className="flex-1 flex items-center justify-center">
            <div className="w-full h-0.5 bg-gray-200"></div>
          </div>
          <div className="w-8 h-8 rounded-full bg-paypal-600 flex items-center justify-center text-white text-sm font-medium">
            3
          </div>
        </div>
        <div className="flex justify-between text-xs text-gray-500 py-2">
          <span>Opened</span>
          <span>Evidence Collected</span>
          <span>Investigating</span>
          <span>Resolution</span>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 space-y-6">
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <div className="flex items-center gap-2 mb-4">
              <div className="w-2 h-2 rounded-full bg-blue-500"></div>
              <h3 className="font-medium text-gray-900">What's happening</h3>
            </div>
            <div className="bg-blue-50 rounded-lg p-4 border border-blue-100">
              <p className="text-gray-800 text-sm">{caseData.summary}</p>
              <p className="mt-2 text-sm font-medium text-blue-800">{caseData.next_step}</p>
            </div>
          </div>

          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <div className="flex items-center gap-2 mb-4">
              <CreditCard className="w-4 h-4 text-gray-500" />
              <h3 className="font-medium text-gray-900">Payment & Order</h3>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="bg-gray-50 rounded-lg p-4 border border-gray-200">
                <p className="text-xs font-medium text-gray-500 uppercase">Payment (PayPal)</p>
                <div className="flex items-baseline gap-1 mt-1">
                  <span className="text-lg font-bold text-gray-900">${(caseData.payment.amount_minor / 100).toFixed(2)}</span>
                  <span className="text-sm font-medium text-gray-500">{caseData.payment.currency}</span>
                </div>
                <div className="flex items-center gap-2 mt-2">
                  <span className={`badge ${caseData.payment.status === 'COMPLETED' ? 'badge-resolved' : 'badge-pending'}`}>
                    {caseData.payment.status}
                  </span>
                </div>
              </div>
              <div className="bg-gray-50 rounded-lg p-4 border border-gray-200">
                <p className="text-xs font-medium text-gray-500 uppercase">Merchant Order</p>
                <div className="text-lg font-bold text-gray-900 mt-1">{caseData.order.ref}</div>
                <div className="flex items-center gap-2 mt-2">
                  <span className={`badge badge-pending ${caseData.order.status === 'PAYMENT_PENDING' ? 'bg-amber-50 text-amber-700 border-amber-200' : 'bg-green-50 text-green-700 border-green-200'}`}>
                    {caseData.order.status}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="lg:col-span-1 space-y-6">
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <h3 className="font-medium text-gray-900 mb-4">Case Timeline</h3>
            <div className="space-y-4">
              {[
                { time: '10:02', text: 'Case opened', type: 'C' },
                { time: '10:02', text: 'Payment confirmed', type: 'S' },
                { time: '10:03', text: 'Merchant notified', type: 'M' },
                { time: '10:04', text: 'Investigation started', type: 'S' },
              ].map((item, idx) => (
                <div key={idx} className="flex items-start gap-3 relative">
                  <div className="relative">
                    <div className={`w-2.5 h-2.5 rounded-full ${getDotColor(item.type)} absolute left-1/2 top-1/2 -mt-1`} />
                    <div className="absolute left-1/2 top-2.5 bottom-2 w-0.5 bg-gray-200" />
                  </div>
                  <div className="text-xs text-gray-500">{item.time}</div>
                  <div className="text-sm text-gray-700">{item.text}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <h3 className="font-medium text-gray-900 mb-4">Your Access</h3>
            <div className="space-y-3">
              <div className="flex items-center gap-2 p-2 rounded-lg bg-blue-50 border border-blue-100">
                <span className="text-xs text-blue-600 font-medium">Can:</span>
                <span className="text-sm text-gray-700">Read, message, add evidence</span>
              </div>
              <div className="flex items-center gap-2 p-2 rounded-lg bg-gray-50 border border-gray-200">
                <span className="text-xs text-gray-500 font-medium">Cannot:</span>
                <span className="text-sm text-gray-600">Approve, refund</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
        <div className="flex items-center justify-between mb-4">
          <h3 className="font-medium text-gray-900">Conversation</h3>
          <span className="text-xs text-gray-500">Shared conversation</span>
        </div>
        <div className="space-y-3 max-h-96 overflow-y-auto pr-2">
          <div className="flex flex-col">
            <div className="text-xs text-gray-400 mb-1">You • 10:02</div>
            <div className="bg-paypal-50 rounded-lg p-3 text-sm text-gray-800 max-w-[80%]">
              I was charged but the store says unpaid.
            </div>
          </div>
          <div className="flex flex-col">
            <div className="text-xs text-gray-400 mb-1 text-right">Merchant • 10:06</div>
            <div className="bg-gray-100 rounded-lg p-3 text-sm text-gray-800 max-w-[80%] ml-auto">
              We are checking your payment with our provider.
            </div>
          </div>
        </div>
        <form onSubmit={handleSendMessage} className="mt-4 flex gap-2">
          <input
            type="text"
            value={newMessage}
            onChange={(e) => setNewMessage(e.target.value)}
            placeholder="Write a message..."
            className="input-field w-full flex-1"
          />
          <button
            type="submit"
            disabled={!newMessage.trim() || loading}
            className="btn-primary px-4"
          >
            Send
          </button>
        </form>
        <button className="mt-3 w-full flex items-center justify-center gap-2 text-sm text-paypal-600 font-medium py-2 bg-blue-50 rounded-lg hover:bg-blue-100 transition-colors">
          <FileText className="w-3 h-3" /> Add Evidence
        </button>
      </div>
    </div>
  );
}

const getDotColor = (type: string) => {
  if (type === 'C') return 'bg-gray-400';
  if (type === 'M') return 'bg-blue-500';
  return 'bg-green-500';
};
