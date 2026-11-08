import { useState, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ArrowLeft, ShieldCheck, Lock, FileText } from '../../components/ui/Icons';
import { apiClient, CustomerCaseView } from '../../services/api';
import { useAuthStore } from '../../store/auth';
import { useSSE } from '../../hooks/useSSE';
import { Badge } from '../components/ui/Badge';
import { Button } from '../components/ui/Button';

interface MerchantCaseView extends CustomerCaseView {
  provider: { name: string; order_id: string; capture_id: string; fetched_at: string };
  investigation: {
    category: string;
    root_cause_hypothesis: string;
    evidence_ids: string[];
    suggested_severity: string;
    validation_status: string;
  };
  latest_decision: { id: string; outcome: string; policy_revision: string; findings: any[] };
  available_actions: string[];
  pending_approvals: any[];
  version: number;
  updated_at: string;
}

export function MerchantCaseDetail() {
  const { id } = useParams();
  const [caseData, setCaseData] = useState<MerchantCaseView | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const { logout } = useAuthStore();
  const navigate = useNavigate();

  const fetchCase = async () => {
    try {
      setLoading(true);
      const data = await apiClient.getCase(id || '');
      setCaseData(data as MerchantCaseView);
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

  const handleReconcile = async () => {
    if (!id) return;
    try {
      await apiClient.proposeAction(id, {
        action_type: 'RECONCILE_MERCHANT_ORDER',
        params: { amount_minor: caseData!.payment.amount_minor, currency: caseData!.payment.currency },
      });
      fetchCase();
    } catch (err: any) {
      setError(err.message || 'Failed to reconcile');
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
          onClick={() => navigate('/merchant/cases')}
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
            onClick={() => navigate('/merchant/cases')}
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

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 space-y-6">
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <div className="flex items-center gap-2 mb-4">
              <CreditCard className="w-4 h-4 text-gray-500" />
              <h3 className="font-medium text-gray-900">Shared State</h3>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="bg-gray-50 rounded-lg p-4 border border-gray-200">
                <p className="text-xs font-medium text-gray-500 uppercase">Provider Order</p>
                <div className="text-sm font-medium text-gray-900">{caseData.provider.order_id}</div>
                <div className="text-xs text-gray-500 mt-1">Capture: {caseData.provider.capture_id}</div>
                <div className="flex items-center gap-2 mt-2">
                  <span className={`badge ${caseData.payment.status === 'COMPLETED' ? 'badge-resolved' : 'badge-pending'}`}>
                    {caseData.payment.status}
                  </span>
                  <span className="text-xs text-gray-500">{caseData.provider.fetched_at.slice(11, 19)}</span>
                </div>
                <div className="flex items-baseline gap-1 mt-2">
                  <span className="text-lg font-bold text-gray-900">${(caseData.payment.amount_minor / 100).toFixed(2)}</span>
                  <span className="text-sm font-medium text-gray-500">{caseData.payment.currency}</span>
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

          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <div className="flex items-center gap-2 mb-4">
              <ShieldCheck className="w-4 h-4 text-blue-500" />
              <h3 className="font-medium text-gray-900">AI Investigation (Advisory)</h3>
            </div>
            <div className="space-y-3">
              <div className="flex justify-between items-start">
                <div>
                  <p className="text-xs font-medium text-gray-500 uppercase">Category</p>
                  <div className="text-sm font-medium text-gray-900">{caseData.investigation.category}</div>
                </div>
                <span className={`badge badge-${caseData.investigation.suggested_severity.toLowerCase()}`}>
                  {caseData.investigation.suggested_severity}
                </span>
              </div>
              <div className="bg-gray-50 rounded-lg p-3 border border-gray-200">
                <p className="text-sm text-gray-800">{caseData.investigation.root_cause_hypothesis}</p>
                <div className="mt-2 flex flex-wrap gap-2">
                  {caseData.investigation.evidence_ids.map((id) => (
                    <span key={id} className="text-xs bg-blue-50 text-blue-700 px-2 py-1 rounded border border-blue-200">
                      {id}
                    </span>
                  ))}
                  <span className="text-xs text-gray-500 italic">(verified)</span>
                </div>
              </div>
            </div>
          </div>

          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <div className="flex items-center gap-2 mb-4">
              <Lock className="w-4 h-4 text-green-500" />
              <h3 className="font-medium text-gray-900">PolicyGuard Decision</h3>
            </div>
            <div className="flex items-center gap-4 py-4 bg-green-50 rounded-lg border border-green-200">
              <div className="flex items-center gap-2">
                <span className="w-2 h-2 rounded-full bg-green-500" />
                <span className="font-bold text-green-700 text-sm">[ ALLOW ]</span>
              </div>
              <div className="text-sm text-gray-900">
                rev {caseData.latest_decision.policy_revision}
              </div>
            </div>
            <div className="space-y-2">
              {caseData.latest_decision.findings.map((f: any, idx: number) => (
                <div key={idx} className="flex items-start gap-2 text-sm">
                  <span className="text-xs font-medium text-green-600">PASS</span>
                  <div className="text-sm text-gray-700">{f.rule_id} {f.code}</div>
                  <div className="text-xs text-gray-500 mt-0.5">{f.message}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <div className="flex items-center gap-2 mb-4">
              <FileText className="w-4 h-4 text-gray-500" />
              <h3 className="font-medium text-gray-900">Actions</h3>
            </div>
            <div className="space-y-3">
              <button
                onClick={handleReconcile}
                disabled={!caseData.available_actions.includes('RECONCILE_MERCHANT_ORDER')}
                className="w-full flex items-center justify-center gap-2 px-4 py-2 border border-gray-200 rounded-lg text-sm text-gray-700 hover:bg-gray-50 transition-colors"
              >
                <Clock className="w-4 h-4 text-gray-500" />
                Reconcile Order
              </button>
              <button className="w-full flex items-center justify-center gap-2 px-4 py-2 border border-gray-200 rounded-lg text-sm text-gray-700 hover:bg-gray-50 transition-colors">
                <ShieldCheck className="w-4 h-4 text-gray-500" />
                Refund (Approval)
              </button>
            </div>
          </div>
        </div>

        <div className="lg:col-span-1 space-y-6">
          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <h3 className="font-medium text-gray-900 mb-4">Audit Timeline</h3>
            <div className="space-y-3">
              {[
                { time: '10:02', text: 'Case opened', type: 'C' },
                { time: '10:02', text: 'Payment confirmed', type: 'S' },
                { time: '10:03', text: 'Merchant notified', type: 'M' },
                { time: '10:04', text: 'Agent run', type: 'M' },
                { time: '10:05', text: 'Decision', type: 'M' },
                { time: '10:06', text: 'Reconciled', type: 'C' },
              ].map((item, idx) => (
                <div key={idx} className="flex items-start gap-3 relative">
                  <div className={`relative w-2.5 h-2.5 rounded-full ${item.type === 'C' ? 'bg-gray-400' : 'bg-blue-500'}`} />
                  <div className="absolute left-1/2 top-2.5 bottom-2 w-0.5 bg-gray-200" />
                  <div className="text-xs text-gray-500">{item.time}</div>
                  <div className="text-sm text-gray-700">{item.text}</div>
                  <span className={`text-xs ${item.type === 'C' ? 'text-blue-600' : 'text-gray-500'} font-medium`}>
                    [{item.type === 'C' ? 'Customer' : 'Merchant'}]
                  </span>
                </div>
              ))}
            </div>
            <div className="text-xs text-gray-500 mt-3">
              [C] Customer-visible &nbsp; [M] Merchant-only
            </div>
          </div>

          <div className="bg-white border border-gray-200 rounded-lg shadow-sm p-6">
            <h3 className="font-medium text-gray-900 mb-4">Your Access (Admin)</h3>
            <div className="space-y-3">
              <div className="flex items-center gap-2 p-2 rounded-lg bg-green-50 border border-green-100">
                <span className="text-xs text-green-600 font-medium">Can:</span>
                <span className="text-sm text-gray-700">Reconcile, approve others' requests, see risk score</span>
              </div>
              <div className="flex items-center gap-2 p-2 rounded-lg bg-gray-50 border border-gray-200">
                <span className="text-xs text-gray-500 font-medium">Cannot:</span>
                <span className="text-sm text-gray-600">Edit policy, see other merchants</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
